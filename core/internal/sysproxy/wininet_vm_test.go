package sysproxy

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// TestWinINetInVM changes the real system proxy of the user, so it runs only in the test machine, where
// POROS_VM_TESTS is set: never on a computer someone works on.
func TestWinINetInVM(t *testing.T) {
	if os.Getenv("POROS_VM_TESTS") != "1" {
		t.Skip("changes the system proxy; runs only in the virtual machine")
	}
	b := winINet{}
	beforeFlags, beforeServer := b.flags(""), b.server("")
	path := filepath.Join(t.TempDir(), "system-proxy.json")

	p := New(path)
	if err := p.Apply("127.0.0.1", 2090); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := b.server(""); got != "127.0.0.1:2090" || b.flags("")&flagProxy == 0 {
		t.Fatalf("after Apply WinINet reads %q, flags %d", got, b.flags(""))
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	enabled, _, _ := key.GetIntegerValue("ProxyEnable")
	server, _, _ := key.GetStringValue("ProxyServer")
	if enabled != 1 || server != "127.0.0.1:2090" {
		t.Errorf("the registry holds ProxyEnable %d, ProxyServer %q", enabled, server)
	}
	if err = p.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if b.flags("")&flagProxy != beforeFlags&flagProxy || b.server("") != beforeServer {
		t.Errorf("Clear left flags %d, server %q; before Apply they were %d, %q", b.flags(""), b.server(""), beforeFlags, beforeServer)
	}

	// A crash: the next core finds the state file and gives the settings back.
	if err = New(path).Apply("127.0.0.1", 2090); err != nil {
		t.Fatal(err)
	}
	if err = New(path).Recover(); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if b.flags("")&flagProxy != beforeFlags&flagProxy || b.server("") != beforeServer {
		t.Errorf("Recover left flags %d, server %q", b.flags(""), b.server(""))
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Recover must remove the state: %v", err)
	}
}
