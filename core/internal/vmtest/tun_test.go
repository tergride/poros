//go:build windows

package vmtest

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/tailscale/go-winio"
	"golang.org/x/sys/windows"

	"PorosCore/config"
	"PorosCore/internal/boxmain"
)

// probeURL answers a fixed text over plain HTTP; Windows itself uses it to tell whether it is online.
const (
	probeURL  = "http://www.msftconnecttest.com/connecttest.txt"
	probeText = "Microsoft Connect Test"
)

func randomHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// counter is a TCP relay in front of the test server that counts connections: the proof that traffic went through
// the server and not around it.
type counter struct {
	listener net.Listener
	count    atomic.Int64
}

func newCounter(t *testing.T, target string) *counter {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c := &counter{listener: listener}
	go func() {
		for {
			in, err := listener.Accept()
			if err != nil {
				return
			}
			c.count.Add(1)
			go func() {
				defer in.Close()
				out, err := net.Dial("tcp", target)
				if err != nil {
					return
				}
				defer out.Close()
				go io.Copy(out, in)
				io.Copy(in, out)
			}()
		}
	}()
	t.Cleanup(func() { listener.Close() })
	return c
}

// shell stands in for Poros.exe: it creates the channel, starts the core and speaks JSON-RPC with it.
type shell struct {
	t        *testing.T
	process  *exec.Cmd
	conn     net.Conn
	mu       sync.Mutex
	nextID   int
	replies  map[int]chan json.RawMessage
	states   chan map[string]any
	networks chan map[string]any
	// downloaded is the last total download of the connection through the server.
	downloaded atomic.Int64
}

func startShell(t *testing.T, program, dataDir string, waitPID int) *shell {
	return startShellWith(t, shellOptions{program: program, dataDir: dataDir, waitPID: waitPID})
}

// shellOptions start a core: hostPID stands for the Poros window, the test by default, and token runs the core
// with fewer rights than the test has.
type shellOptions struct {
	program, dataDir string
	waitPID, hostPID int
	token            windows.Token
}

func startShellWith(t *testing.T, o shellOptions) *shell {
	t.Helper()
	name := `\\.\pipe\poros-vmtest-` + randomHex(t, 8)
	token := randomHex(t, 16)
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	// The account itself, not only its administrators: a core without rights connects too.
	listener, err := winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;" + user.User.Sid.String() + ")",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	hostPID := o.hostPID
	if hostPID == 0 {
		hostPID = os.Getpid()
	}
	args := []string{"--pipe", name, "--token", token, "--data-dir", o.dataDir, "--host-pid", strconv.Itoa(hostPID)}
	if o.waitPID != 0 {
		args = append(args, "--wait-pid", strconv.Itoa(o.waitPID))
	}
	process := exec.Command(o.program, args...)
	if o.token != 0 {
		process.SysProcAttr = &syscall.SysProcAttr{Token: syscall.Token(o.token)}
	}
	if err = process.Start(); err != nil {
		t.Fatal(err)
	}
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, _ := listener.Accept()
		accepted <- conn
	}()
	var conn net.Conn
	select {
	case conn = <-accepted:
	case <-time.After(45 * time.Second):
		process.Process.Kill()
		t.Fatal("the core did not connect")
	}
	s := &shell{
		t: t, process: process, conn: conn, replies: map[int]chan json.RawMessage{},
		states: make(chan map[string]any, 64), networks: make(chan map[string]any, 64),
	}
	go s.read()
	s.call("hello", map[string]string{"token": token})
	return s
}

func (s *shell) read() {
	scanner := bufio.NewScanner(s.conn)
	scanner.Buffer(make([]byte, 1<<20), 16<<20)
	for scanner.Scan() {
		var message struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(scanner.Bytes(), &message) != nil {
			continue
		}
		if message.ID != nil {
			s.mu.Lock()
			reply := s.replies[*message.ID]
			s.mu.Unlock()
			if message.Error != nil {
				reply <- append(json.RawMessage(`{"error":`), append(message.Error, '}')...)
			} else {
				reply <- message.Result
			}
			continue
		}
		var state map[string]any
		json.Unmarshal(message.Params, &state)
		switch message.Method {
		case "connection.changed":
			s.states <- state
		case "network.changed":
			s.networks <- state
		case "connection.traffic":
			if total, ok := state["totalDownload"].(float64); ok {
				s.downloaded.Store(int64(total))
			}
		}
	}
}

func (s *shell) call(method string, params any) json.RawMessage {
	s.t.Helper()
	result, failure := s.request(method, params)
	if failure != "" {
		s.t.Fatalf("%s: %s", method, failure)
	}
	return result
}

// request returns the result of a call, or the error message of the core for the caller to print or hide.
func (s *shell) request(method string, params any) (json.RawMessage, string) {
	s.t.Helper()
	s.mu.Lock()
	id := s.nextID
	s.nextID++
	reply := make(chan json.RawMessage, 1)
	s.replies[id] = reply
	s.mu.Unlock()
	data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if _, err := s.conn.Write(append(data, '\n')); err != nil {
		s.t.Fatalf("%s: %v", method, err)
	}
	select {
	case result := <-reply:
		var failed struct {
			Error *struct{ Message string } `json:"error"`
		}
		if json.Unmarshal(result, &failed) == nil && failed.Error != nil {
			return nil, failed.Error.Message
		}
		return result, ""
	case <-time.After(60 * time.Second):
		s.t.Fatalf("%s: no answer", method)
	}
	return nil, ""
}

// connect starts the connection and waits until it runs; it returns how long that took.
func (s *shell) connect() time.Duration {
	s.t.Helper()
	started := time.Now()
	if state := s.start(); state["state"] != "running" {
		s.t.Fatalf("the connection did not come up: %v %v", state["error"], state["errorText"])
	}
	return time.Since(started)
}

// start starts the connection and returns its state once it runs or has failed.
func (s *shell) start() map[string]any {
	s.t.Helper()
	s.call("connection.start", nil)
	deadline := time.After(90 * time.Second)
	for {
		select {
		case state := <-s.states:
			if state["state"] == "running" || state["state"] == "idle" {
				return state
			}
		case <-deadline:
			s.t.Fatal("the connection did not come up in 90 seconds")
		}
	}
}

// fetch opens the probe page the way any program does: no proxy, so through the TUN.
func fetch(timeout time.Duration) error {
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		response, err := client.Get(probeURL)
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if string(body) == probeText {
				return nil
			}
			err = fmt.Errorf("unexpected answer %q", body)
		}
		last = err
		time.Sleep(time.Second)
	}
	return last
}

func tunPresent() bool {
	_, err := net.InterfaceByName(config.TunName)
	return err == nil
}

// TestTUNAfterAKilledCore connects through the TUN, kills the core with the TUN up, as a crash does, and checks
// that the next core connects again: the user's note that a core restart with TUN hung the connection.
func TestTUNAfterAKilledCore(t *testing.T) {
	program := os.Getenv("POROS_TEST_CORE")
	if os.Getenv("POROS_VM_TESTS") != "1" || program == "" {
		t.Skip("brings up a TUN; runs only in the virtual machine")
	}
	dataDir := t.TempDir()
	relay, providerURL := startTestServer(t)
	first := startShell(t, program, dataDir, 0)
	first.useServer(providerURL)

	took := first.connect()
	if !tunPresent() {
		t.Fatal("the TUN adapter is missing while the connection runs")
	}
	before := relay.count.Load()
	if err := fetch(30 * time.Second); err != nil {
		t.Fatalf("the page does not open through the TUN: %v", err)
	}
	if relay.count.Load() == before {
		t.Fatal("the page opened around the server, not through it")
	}
	t.Logf("first core: connected in %v", took)

	// A crash: the process goes with the TUN up and nothing closes it.
	killedPID := first.process.Process.Pid
	first.process.Process.Kill()
	first.process.Wait()
	first.conn.Close()
	t.Logf("killed the core; TUN adapter still there: %v", tunPresent())

	killed := time.Now()
	second := startShell(t, program, dataDir, killedPID)
	took = second.connect()
	before = relay.count.Load()
	if err := fetch(30 * time.Second); err != nil {
		t.Fatalf("after the killed core the page does not open again: %v", err)
	}
	if relay.count.Load() == before {
		t.Fatal("after the restart the page opened around the server")
	}
	t.Logf("second core: connected in %v, page open %v after the kill", took, time.Since(killed))

	// A clean exit: the shell closes the channel, the core closes the TUN and leaves.
	second.conn.Close()
	exited := make(chan error, 1)
	go func() { exited <- second.process.Wait() }()
	select {
	case <-exited:
	case <-time.After(25 * time.Second):
		second.process.Process.Kill()
		t.Fatal("the core did not exit after its channel closed")
	}
	deadline := time.Now().Add(15 * time.Second)
	for tunPresent() && time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
	}
	if tunPresent() {
		t.Error("the TUN adapter stayed after a clean exit")
	}
	if err := fetch(20 * time.Second); err != nil {
		t.Errorf("after the clean exit the network does not work without Poros: %v", err)
	}
}

// startTestServer runs a Trojan server behind a counting relay and a subscription with it, all on loopback. The
// server binds its own connections to the network adapter, or they would loop back into the TUN, and resolves
// names over DoH: strict_route lets no other process than Poros's core send DNS past the TUN.
func startTestServer(t *testing.T) (*counter, string) {
	t.Helper()
	serverPort := freePort(t)
	server, cancel, err := boxmain.Create([]byte(fmt.Sprintf(`{"log":{"level":"warn"},`+
		`"dns":{"servers":[{"type":"https","tag":"doh","server":"1.1.1.1"}]},`+
		`"inbounds":[{"type":"trojan","listen":"127.0.0.1","listen_port":%d,"users":[{"password":"secret"}]}],`+
		`"outbounds":[{"type":"direct","tag":"direct"}],"route":{"auto_detect_interface":true,"default_domain_resolver":"doh"}}`,
		serverPort)), nil)
	if err != nil {
		t.Fatalf("the test server: %v", err)
	}
	t.Cleanup(func() { cancel(); server.Close() })
	relay := newCounter(t, fmt.Sprintf("127.0.0.1:%d", serverPort))
	link := fmt.Sprintf("trojan://secret@%s?security=none#vm", relay.listener.Addr())
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, link) }))
	t.Cleanup(provider.Close)
	return relay, provider.URL + "/sub"
}

// useServer adds the subscription and chooses its server, with TUN and the preset that sends all through it.
func (s *shell) useServer(providerURL string) {
	s.t.Helper()
	var update struct {
		Group struct{ ID string } `json:"group"`
	}
	json.Unmarshal(s.call("subscriptions.add", map[string]string{"url": providerURL, "name": ""}), &update)
	var servers []struct{ ID string }
	json.Unmarshal(s.call("servers.list", map[string]string{"groupId": update.Group.ID}), &servers)
	if len(servers) != 1 {
		s.t.Fatalf("servers: %v", servers)
	}
	var settings map[string]any
	json.Unmarshal(s.call("settings.get", nil), &settings)
	settings["server"], settings["tun"], settings["preset"] = servers[0].ID, true, "all"
	s.call("settings.set", settings)
}

// TestNetworkCheckThroughTUN asks the outside services who sees the machine, with and without the VPN, and runs
// the DNS leak test with the TUN up. The addresses are not printed: through the host they are the host's.
func TestNetworkCheckThroughTUN(t *testing.T) {
	program := os.Getenv("POROS_TEST_CORE")
	if os.Getenv("POROS_VM_TESTS") != "1" || program == "" {
		t.Skip("brings up a TUN and asks outside services; runs only in the virtual machine")
	}
	_, providerURL := startTestServer(t)
	s := startShell(t, program, t.TempDir(), 0)
	defer s.conn.Close()
	s.useServer(providerURL)
	s.connect()
	s.call("network.check", nil)
	deadline := time.After(90 * time.Second)
	for {
		select {
		case state := <-s.networks:
			if state["checking"] == true {
				continue
			}
			direct, _ := state["direct"].(map[string]any)
			proxy, _ := state["proxy"].(map[string]any)
			dns, _ := state["dns"].([]any)
			t.Logf("without the VPN: found %v, %v, AS%v; through the VPN: found %v, %v, AS%v; DNS servers: %d; leak %q; %v",
				direct["ip"] != "", direct["countryCode"], direct["asn"], proxy["ip"] != "", proxy["countryCode"], proxy["asn"],
				len(dns), state["leak"], state["errorText"])
			if direct["ip"] == "" || proxy["ip"] == "" || len(dns) == 0 {
				t.Fatalf("the check is incomplete: %v", state["errorText"])
			}
			if state["leak"] != "none" {
				t.Fatalf("with the TUN up the DNS must not leak: %q", state["leak"])
			}
			return
		case <-deadline:
			t.Fatal("the check did not finish in 90 seconds")
		}
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
