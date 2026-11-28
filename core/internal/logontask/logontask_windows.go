//go:build windows

package logontask

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// Name is the task of the copy of Poros whose core is at core: every portable copy had its own.
func Name(core string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(filepath.Dir(core)))))
	return "Poros core at sign-in " + hex.EncodeToString(sum[:4])
}

// Remove deletes the task of this copy; one that is not there is fine. It takes administrator rights.
func Remove(core, dataDir string) error {
	if schtasks("/Query", "/TN", Name(core)) == nil {
		if err := schtasks("/Delete", "/TN", Name(core), "/F"); err != nil {
			return err
		}
	}
	clearRemoval(dataDir)
	return nil
}

// Exists tells whether the task of this copy is there. A user without rights may not see a task an administrator
// made, so false is not sure without rights.
func Exists(core string) bool {
	return schtasks("/Query", "/TN", Name(core)) == nil
}

func schtasks(args ...string) error {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(system, "schtasks.exe"), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks %s: %w: %s", args[0], err, strings.TrimSpace(string(output)))
	}
	return nil
}
