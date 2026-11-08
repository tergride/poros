package logontask

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitLaunchTakesTheShellsFile(t *testing.T) {
	dir := t.TempDir()
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, launchFile), []byte(`{"pipe":"\\\\.\\pipe\\x","token":"t","hostPid":42,"version":"0.1.0"}`), 0o644)
	}()
	launch, err := WaitLaunch(dir, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if launch.Pipe != `\\.\pipe\x` || launch.Token != "t" || launch.HostPID != 42 || launch.Version != "0.1.0" {
		t.Errorf("launch %+v", launch)
	}
	if _, err = os.Stat(filepath.Join(dir, launchFile)); !os.IsNotExist(err) {
		t.Error("the file stayed")
	}
	if _, err = WaitLaunch(dir, 300*time.Millisecond); err == nil {
		t.Error("a second wait found a shell")
	}
}
