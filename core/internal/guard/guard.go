// Package guard is the kill switch: a process with administrator rights that blocks every packet not sent by
// Poros's core or routed into its TUN. An elevated core starts it with --guard for its Poros window, and it lives as
// long as that window, not the core: the block outlasts a core that fails or is replaced, and any core of the
// window, with rights or without, finds the guard by the window's process and turns the block off through its pipe.
package guard

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/logger"

	corehost "PorosCore/internal/host"
)

const (
	CodePrivilege   = "privilege"
	CodeUnsupported = "unsupported"
	CodeConfig      = "config"
	CodeTampered    = "tampered"
	CodeInternal    = "internal"
)

// Config is the block a guard holds.
type Config struct {
	// AllowLAN lets traffic to the local network pass, except DNS.
	AllowLAN bool
	// TunName is the alias of the TUN adapter whose traffic passes; empty passes none.
	TunName string
}

type config struct {
	AllowLAN bool
	TunName  string
}

// Event is a runtime report from a backend; a Fatal one makes the guard disarm and exit.
type Event struct {
	Fatal   bool
	Code    string
	Message string
}

type backend interface {
	// Arm installs the whole ruleset atomically and returns only once it is enforced; on error nothing stays installed.
	Arm(cfg *config, events chan<- Event) error
	// Disarm removes everything Arm installed; it must be safe to call more than once and after a failed Arm.
	Disarm()
}

type Error struct {
	Code string
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

func newError(code string, format string, args ...any) error {
	return &Error{Code: code, Err: fmt.Errorf(format, args...)}
}

// ErrorCode is the code of a guard error, CodeInternal for any other.
func ErrorCode(err error) string {
	var guardErr *Error
	if errors.As(err, &guardErr) {
		return guardErr.Code
	}
	return CodeInternal
}

var (
	lanPrefixes4 = []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("169.254.0.0/16"),
		netip.MustParsePrefix("224.0.0.0/4"),
		netip.MustParsePrefix("255.255.255.255/32"),
	}
	lanPrefixes6 = []netip.Prefix{
		netip.MustParsePrefix("fe80::/10"),
		netip.MustParsePrefix("fc00::/7"),
		netip.MustParsePrefix("ff00::/8"),
	}
	// Blocked towards LAN even when LAN is allowed: a LAN resolver forwards upstream, outside the tunnel.
	lanDNSPorts = []uint16{53, 853}
)

var interfaceNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]*$`)

func newConfig(c Config) (*config, error) {
	if len(c.TunName) > 15 || !interfaceNamePattern.MatchString(c.TunName) {
		return nil, newError(CodeConfig, "invalid tun name %q", c.TunName)
	}
	return &config{AllowLAN: c.AllowLAN, TunName: c.TunName}, nil
}

// options are the guard's arguments: the window it serves and the log it writes.
type options struct {
	hostPID int
	logPath string
	config  Config
}

func parseOptions(args []string) (options, error) {
	var opts options
	flags := flag.NewFlagSet("guard", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.IntVar(&opts.hostPID, "host-pid", 0, "")
	flags.StringVar(&opts.logPath, "log", "", "")
	flags.BoolVar(&opts.config.AllowLAN, "allow-lan", false, "")
	flags.StringVar(&opts.config.TunName, "tun-name", "", "")
	if err := flags.Parse(args); err != nil {
		return opts, newError(CodeConfig, "arguments: %v", err)
	}
	if opts.hostPID <= 0 {
		return opts, newError(CodeConfig, "--host-pid is required")
	}
	return opts, nil
}

func (o options) args() []string {
	args := []string{"--guard", "--host-pid", fmt.Sprint(o.hostPID)}
	if o.logPath != "" {
		args = append(args, "--log", o.logPath)
	}
	if o.config.AllowLAN {
		args = append(args, "--allow-lan")
	}
	if o.config.TunName != "" {
		args = append(args, "--tun-name", o.config.TunName)
	}
	return args
}

// Lines of the guard's stdout before the block is on, and of its pipe after: ON greets a core that joins, a core
// may ask to disarm, and WARN and FAIL report. The guard closes the pipe once the block is off.
const (
	lineReady  = "READY"
	lineError  = "ERROR"
	lineOn     = "ON"
	lineWarn   = "WARN"
	lineFail   = "FAIL"
	lineDisarm = "disarm"
)

func clean(text string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(text)
}

// openLog appends to the guards' log, so that a guard keeps what the ones before it wrote.
func openLog(path string) *os.File {
	if path == "" {
		return nil
	}
	f, err := corehost.OpenLog(path)
	if err != nil {
		return nil
	}
	return f
}

// Run never returns. Until the block is on, stdout carries one line: READY, or ERROR <code> <message>. From then on
// the guard speaks only through its pipe, since the core that started it may be gone.
func Run(args []string) {
	stdout := os.Stdout
	fail := func(err error) {
		log.Printf("guard: %v", err)
		fmt.Fprintln(stdout, lineError, ErrorCode(err), clean(err.Error()))
		os.Exit(1)
	}
	opts, err := parseOptions(args)
	if err != nil {
		fail(err)
	}
	if logFile := openLog(opts.logPath); logFile != nil {
		log.SetOutput(logFile)
		redirectStderr(logFile)
		_ = debug.SetCrashOutput(logFile, debug.CrashOptions{})
	}
	cfg, err := newConfig(opts.config)
	if err != nil {
		fail(err)
	}
	host, err := openHost(opts.hostPID)
	if err != nil {
		fail(err)
	}

	var active backend
	defer func() {
		if r := recover(); r != nil {
			if active != nil {
				active.Disarm()
			}
			log.Printf("guard panicked: %v\n%s", r, debug.Stack())
			os.Exit(2)
		}
	}()
	events := make(chan Event, 16)
	active, err = newBackend()
	if err == nil {
		err = active.Arm(cfg, events)
	}
	if err != nil {
		if active != nil {
			active.Disarm()
		}
		fail(err)
	}
	listener, err := listen(PipeName(opts.hostPID), pipeSecurity(host.userSID))
	if err != nil {
		active.Disarm()
		fail(newError(CodeInternal, "open the guard's pipe: %v", err))
	}
	log.Printf("guard: on for the Poros window %d", opts.hostPID)
	fmt.Fprintln(stdout, lineReady)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	stop := make(chan string, 1)
	go func() {
		select {
		case <-host.gone:
			stop <- "the Poros window is gone"
		case received := <-signals:
			stop <- received.String()
		}
	}()
	os.Exit(serve(active, listener, events, stop))
}

// server answers the cores on the guard's pipe: each may turn the block off, and each hears when it fails.
type server struct {
	mu      sync.Mutex
	clients map[net.Conn]struct{}
	closed  bool
	disarm  chan struct{}
}

// accept greets each core under the lock, so that it hears every report that comes after it joined.
func (s *server) accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			conn.Close()
			continue
		}
		s.clients[conn] = struct{}{}
		s.send(conn, lineOn)
		s.mu.Unlock()
		go s.read(conn)
	}
}

func (s *server) read(conn net.Conn) {
	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
	}()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != lineDisarm {
			s.mu.Lock()
			s.send(conn, lineError+" "+CodeConfig+" unknown request")
			s.mu.Unlock()
			continue
		}
		select {
		case s.disarm <- struct{}{}:
		default:
		}
	}
}

func (s *server) send(conn net.Conn, line string) {
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
	_, _ = io.WriteString(conn, line+"\n")
}

// closeAll drops every core: the block is off by then, and the pipe goes with the last of them.
func (s *server) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for conn := range s.clients {
		conn.Close()
	}
}

func (s *server) broadcast(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for conn := range s.clients {
		s.send(conn, line)
	}
}

// serve holds the block until a core turns it off, stop comes or the backend fails, and returns the exit code.
func serve(active backend, listener net.Listener, events <-chan Event, stop <-chan string) int {
	s := &server{clients: map[net.Conn]struct{}{}, disarm: make(chan struct{}, 1)}
	go s.accept(listener)
	defer func() {
		// No core joins any more, then the cores hear the guard go. go-winio's Close can hang when a connect races
		// it; the process exits right after, which closes the pipe for good.
		closed := make(chan struct{})
		go func() {
			listener.Close()
			close(closed)
		}()
		select {
		case <-closed:
		case <-time.After(time.Second):
		}
		s.closeAll()
	}()
	for {
		select {
		case reason := <-stop:
			log.Printf("guard: %s, disarming", reason)
			active.Disarm()
			return 0
		case <-s.disarm:
			log.Print("guard: a core turned the block off")
			active.Disarm()
			return 0
		case event := <-events:
			if !event.Fatal {
				log.Printf("guard: %s", event.Message)
				s.broadcast(lineWarn + " " + clean(event.Message))
				continue
			}
			log.Printf("guard: %s: %s, disarming", event.Code, event.Message)
			s.broadcast(lineFail + " " + event.Code + " " + clean(event.Message))
			active.Disarm()
			return 3
		}
	}
}

// goSafe runs fn on its own goroutine; a panic there must disarm through serve, not kill the process with rules installed.
func goSafe(events chan<- Event, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("guard: panic: %v\n%s", r, debug.Stack())
				events <- Event{Fatal: true, Code: CodeInternal, Message: fmt.Sprint("internal error: ", r)}
			}
		}()
		fn()
	}()
}

// watchInterfaces calls onChange after every interface or route change, and every interval as a fallback.
func watchInterfaces(stop <-chan struct{}, interval time.Duration, onChange func()) {
	trigger := make(chan struct{}, 1)
	notify := func() {
		select {
		case trigger <- struct{}{}:
		default:
		}
	}
	monitor, err := tun.NewNetworkUpdateMonitor(logger.NOP())
	if err == nil {
		monitor.RegisterCallback(notify)
		if err = monitor.Start(); err != nil {
			log.Printf("guard: network monitor unavailable, polling only: %v", err)
		} else {
			defer monitor.Close()
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-trigger:
			onChange()
		case <-ticker.C:
			onChange()
		}
	}
}
