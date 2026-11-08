//go:build windows

package vmtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTUNAfterAPowerCut runs in two phases around a power cut that the script of the machine makes. "hold"
// connects through the TUN and waits for the machine to be turned off with it up; Windows then keeps the TUN's device
// as one that is not present. "start", after the boot, connects again: the core removes that device first and the
// TUN comes up at once. On a user's Windows 10 bringing such a device back stalled the TUN and failed the start.
func TestTUNAfterAPowerCut(t *testing.T) {
	program, phase := os.Getenv("POROS_TEST_CORE"), os.Getenv("POROS_TEST_PHASE")
	if os.Getenv("POROS_VM_TESTS") != "1" || program == "" || phase == "" {
		t.Skip("needs the machine powered off with a TUN up; runs only from the power cut script of the virtual machine")
	}
	dataDir := filepath.Join(os.Getenv("ProgramData"), "poros-powercut-"+phase)
	os.RemoveAll(dataDir)
	_, providerURL := startTestServer(t)
	s := startShell(t, program, dataDir, 0)
	s.useServer(providerURL)
	if phase == "hold" {
		s.connect()
		if !tunPresent() {
			t.Fatal("the TUN adapter is missing while the connection runs")
		}
		os.WriteFile(filepath.Join(os.Getenv("ProgramData"), "poros-powercut-held"), nil, 0o644)
		time.Sleep(30 * time.Minute)
		return
	}
	took := s.connect()
	t.Logf("after the power cut: connected in %v", took)
	if err := fetch(30 * time.Second); err != nil {
		t.Fatalf("the page does not open through the TUN: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dataDir, "logs", "core.log"))
	if !strings.Contains(string(data), "removed the TUN adapter an earlier run left behind") {
		t.Error("the core did not remove the TUN device the power cut left")
	}
	s.call("connection.stop", nil)
	s.waitIdle()
	s.exit(t)
}
