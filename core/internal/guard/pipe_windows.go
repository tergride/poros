package guard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tailscale/go-winio"
	"golang.org/x/sys/windows"
)

const (
	dialTimeout   = 2 * time.Second
	readyTimeout  = 15 * time.Second
	disarmTimeout = 10 * time.Second
)

// PipeName is the pipe of the guard of one Poros window.
func PipeName(hostPID int) string {
	return `\\.\pipe\poros-guard-` + strconv.Itoa(hostPID)
}

// pipeSecurity opens the guard's pipe to the account of its window: that account may read and write but not create
// an instance of its own (FILE_CREATE_PIPE_INSTANCE is left out), so no program of the user can stand in for the
// guard. Administrators and the system have it all, the guard among them.
func pipeSecurity(userSID string) string {
	return fmt.Sprintf("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x12019b;;;%s)", userSID)
}

func listen(name, sddl string) (net.Listener, error) {
	return winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: sddl})
}

// clientAccess asks for reading and writing without GENERIC_WRITE, which for a pipe includes the creation of an
// instance that pipeSecurity withholds.
const clientAccess = windows.GENERIC_READ | windows.FILE_WRITE_DATA

// host is the Poros window the guard serves.
type host struct {
	gone    chan struct{}
	userSID string
}

func openHost(pid int) (*host, error) {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return nil, newError(CodeConfig, "the Poros window %d is not running: %v", pid, err)
	}
	sid, err := processUser(process)
	if err != nil {
		windows.CloseHandle(process)
		return nil, newError(CodeInternal, "the account of the Poros window: %v", err)
	}
	h := &host{gone: make(chan struct{}), userSID: sid}
	go func() {
		defer windows.CloseHandle(process)
		windows.WaitForSingleObject(process, windows.INFINITE)
		close(h.gone)
	}()
	return h, nil
}

func processUser(process windows.Handle) (string, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return "", err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

// redirectStderr sends the guard's standard error to its log, where a panic of the runtime then lands too.
func redirectStderr(f *os.File) {
	_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))
	os.Stderr = f
}

// Link is a core's line to a guard whose block is on. The block stays when the link closes.
type Link struct {
	conn   net.Conn
	reader *bufio.Reader
	lost   chan struct{}

	mu     sync.Mutex
	reason string
}

// Find joins the guard of the Poros window hostPID; it is nil when that window has none.
func Find(hostPID int) (*Link, error) {
	return dial(PipeName(hostPID))
}

func dial(name string) (*Link, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	conn, err := winio.DialPipeAccess(ctx, name, clientAccess)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("join the kill switch: %w", err)
	}
	if err = verifyServer(conn); err != nil {
		conn.Close()
		return nil, err
	}
	// The greeting tells that the guard counts this core in; a guard on its way out closes the pipe instead.
	reader := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(dialTimeout))
	line, err := reader.ReadString('\n')
	_ = conn.SetReadDeadline(time.Time{})
	if errors.Is(err, io.EOF) {
		conn.Close()
		return nil, nil
	}
	if err != nil || strings.TrimSpace(line) != lineOn {
		conn.Close()
		return nil, fmt.Errorf("the kill switch did not answer: %q, %v", line, err)
	}
	l := &Link{conn: conn, reader: reader, lost: make(chan struct{})}
	go l.read()
	return l, nil
}

// verifyServer makes sure the pipe is held by this very program: any other would only pretend the block is on.
func verifyServer(conn net.Conn) error {
	file, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return errors.New("the kill switch pipe has no handle")
	}
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(windows.Handle(file.Fd()), &pid); err != nil {
		return fmt.Errorf("the process of the kill switch pipe: %w", err)
	}
	image, err := processImage(pid)
	if err != nil {
		return fmt.Errorf("the program of the kill switch pipe: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(image), filepath.Clean(self)) {
		return fmt.Errorf("the kill switch pipe is held by %s, not by Poros", image)
	}
	return nil
}

func processImage(pid uint32) (string, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(process)
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buffer[:size]), nil
}

func (l *Link) read() {
	defer close(l.lost)
	scanner := bufio.NewScanner(l.reader)
	for scanner.Scan() {
		verb, rest, _ := strings.Cut(strings.TrimSpace(scanner.Text()), " ")
		if verb == lineFail {
			_, message, _ := strings.Cut(rest, " ")
			l.mu.Lock()
			l.reason = message
			l.mu.Unlock()
		}
	}
}

// Lost is closed when the block goes away: turned off, or failed, and then Reason says why.
func (l *Link) Lost() <-chan struct{} { return l.lost }

// Reason is why the block failed, "" when it was turned off or its process just ended.
func (l *Link) Reason() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.reason
}

// Disarm turns the block off and returns once the guard has let go of its pipe, so a new guard can start.
func (l *Link) Disarm() error {
	defer l.conn.Close()
	_ = l.conn.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := io.WriteString(l.conn, lineDisarm+"\n"); err != nil {
		select {
		case <-l.lost:
			return nil
		default:
			return fmt.Errorf("turn the kill switch off: %w", err)
		}
	}
	select {
	case <-l.lost:
		return nil
	case <-time.After(disarmTimeout):
		return fmt.Errorf("the kill switch did not turn off in %v", disarmTimeout)
	}
}

// Close lets go of the block; it stays on.
func (l *Link) Close() error {
	return l.conn.Close()
}

// Start starts a guard for the Poros window hostPID and joins it once its block is on; a guard already there is
// joined instead. It needs administrator rights, and the guard writes its log to logPath.
func Start(hostPID int, logPath string, cfg Config) (*Link, error) {
	if link, err := Find(hostPID); link != nil || err != nil {
		return link, err
	}
	if _, err := newConfig(cfg); err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, newError(CodeInternal, "locate PorosCore: %v", err)
	}
	cmd := exec.Command(self, options{hostPID: hostPID, logPath: logPath, config: cfg}.args()...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, newError(CodeInternal, "start the kill switch: %v", err)
	}
	if err = cmd.Start(); err != nil {
		return nil, newError(CodeInternal, "start the kill switch: %v", err)
	}
	answer := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		answer <- strings.TrimSpace(line)
	}()
	var line string
	select {
	case line = <-answer:
	case <-time.After(readyTimeout):
		cmd.Process.Kill()
		cmd.Wait()
		return nil, newError(CodeInternal, "the kill switch did not answer in %v", readyTimeout)
	}
	if line != lineReady {
		waitErr := cmd.Wait()
		verb, rest, _ := strings.Cut(line, " ")
		if verb != lineError {
			return nil, newError(CodeInternal, "the kill switch stopped before it was on: %v", waitErr)
		}
		code, message, _ := strings.Cut(rest, " ")
		return nil, newError(code, "%s", message)
	}
	// From here the guard lives on its own: the core neither waits for it nor reads its output.
	stdout.Close()
	cmd.Process.Release()
	link, err := Find(hostPID)
	if err == nil && link == nil {
		err = newError(CodeInternal, "the kill switch is on, but its pipe is missing")
	}
	return link, err
}
