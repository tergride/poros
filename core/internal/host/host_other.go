//go:build !windows

package host

import (
	"errors"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// WaitExit waits for a process to end; one that is already gone, or was never there, has ended.
func WaitExit(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for alive(pid) {
		if time.Now().After(deadline) {
			return errors.New("the previous core did not exit in time")
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

// Watch closes the channel when the process ends.
func Watch(pid int) <-chan struct{} {
	gone := make(chan struct{})
	go func() {
		for alive(pid) {
			time.Sleep(time.Second)
		}
		close(gone)
	}()
	return gone
}

// SameUser tells whether a process runs as the same account as this one.
func SameUser(pid int) (bool, error) {
	info, err := os.Stat("/proc/" + strconv.Itoa(pid))
	if err != nil {
		return false, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid(), nil
}

// RedirectOutput sends this process's standard output and error to f.
func RedirectOutput(f *os.File) error {
	if err := unix.Dup2(int(f.Fd()), 1); err != nil {
		return err
	}
	if err := unix.Dup2(int(f.Fd()), 2); err != nil {
		return err
	}
	os.Stdout, os.Stderr = f, f
	return nil
}

// Elevated tells whether this process runs as root.
func Elevated() bool {
	return os.Geteuid() == 0
}

// SystemVersion names the system for the log.
func SystemVersion() string {
	return runtime.GOOS
}
