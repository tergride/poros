// Package host follows the processes around the core: the shell that started it, and a previous core that must
// let go of the data folder, the port and the TUN first.
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

// SameUser tells whether a process runs as the same account as this one. A core elevated under another
// administrator's account would otherwise set that account's system proxy.
func SameUser(pid int) (bool, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(process)
	var token windows.Token
	if err = windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return false, err
	}
	defer token.Close()
	theirs, err := token.GetTokenUser()
	if err != nil {
		return false, err
	}
	ours, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return false, err
	}
	return windows.EqualSid(theirs.User.Sid, ours.User.Sid), nil
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
