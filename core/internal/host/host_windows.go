// Package host follows the processes around the core: the shell that started it, and a previous core that must
// let go of the data folder, the port and the TUN first. Their accounts may differ: UAC elevates a core under the
// account of the administrator who gave the rights.
package host

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// WaitExit waits for a process to end; one that is already gone, or was never there, has ended.
func WaitExit(pid int, timeout time.Duration) error {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(process)
	event, err := windows.WaitForSingleObject(process, uint32(timeout.Milliseconds()))
	if err != nil {
		return err
	}
	if event == uint32(windows.WAIT_TIMEOUT) {
		return errors.New("the previous core did not exit in time")
	}
	return nil
}

// Watch closes the channel when the process ends.
func Watch(pid int) <-chan struct{} {
	gone := make(chan struct{})
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		close(gone)
		return gone
	}
	go func() {
		defer windows.CloseHandle(process)
		windows.WaitForSingleObject(process, windows.INFINITE)
		close(gone)
	}()
	return gone
}

// administrators is the group of the administrators of the computer.
const administrators = "S-1-5-32-544"

// User is the account of this process, as the SID.
func User() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

// OpenTo lets the administrators and the account user, "" for none, wait for this process and read which program it
// runs. The access list Windows gives a process lets in only its own account: a core that UAC elevated under another
// account than its window's could otherwise neither watch the window nor wait for the core it replaces, and the
// window's next core could not wait for it or check its guard.
func OpenTo(user string) error {
	sids := []string{administrators}
	if user != "" {
		sids = append(sids, user)
	}
	entries := make([]windows.EXPLICIT_ACCESS, 0, len(sids))
	for _, text := range sids {
		sid, err := windows.StringToSid(text)
		if err != nil {
			return fmt.Errorf("the account %s: %w", text, err)
		}
		entries = append(entries, windows.EXPLICIT_ACCESS{
			AccessPermissions: windows.SYNCHRONIZE | windows.PROCESS_QUERY_LIMITED_INFORMATION,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_UNKNOWN,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		})
	}
	process := windows.CurrentProcess()
	descriptor, err := windows.GetSecurityInfo(process, windows.SE_KERNEL_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	current, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	dacl, err := windows.ACLFromEntries(entries, current)
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(process, windows.SE_KERNEL_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// RedirectOutput sends this process's standard output and error to f: an elevated core has no pipe to its shell,
// and Xray and sing-box write their logs there.
func RedirectOutput(f *os.File) error {
	handle := windows.Handle(f.Fd())
	if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, handle); err != nil {
		return err
	}
	if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, handle); err != nil {
		return err
	}
	os.Stdout, os.Stderr = f, f
	return nil
}

// Elevated tells whether this process has administrator rights.
func Elevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// SystemVersion names the system for the log.
func SystemVersion() string {
	major, minor, build := windows.RtlGetNtVersionNumbers()
	return fmt.Sprintf("Windows %d.%d.%d", major, minor, build)
}
