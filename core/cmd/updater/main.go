// The updater of the portable build: replaces the program files next to it with a release archive, then starts Poros
// again. Poros runs a copy named updater.old, so the archive can replace updater.exe as well.
package main

import (
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"PorosCore/internal/release"
)

const (
	// Poros reads this next to the archive on its next start and tells the user: a reason on the first line, the
	// details after it.
	errorFile = "install-error.txt"
	exitWait  = 30 * time.Second
	// PorosCore.exe outlives Poros for a moment, and a virus scanner holds a new file for a while.
	lockWait = 30 * time.Second
)

// Reasons of a failed install.
const (
	// reasonDamaged is an archive that is no release this updater installs.
	reasonDamaged = "damaged"
	// reasonBusy is a file of Poros that could not be replaced, most likely held by another program.
	reasonBusy   = "busy"
	reasonFailed = "failed"
)

func reasonOf(err error) string {
	switch {
	case errors.Is(err, release.ErrDamaged):
		return reasonDamaged
	case errors.Is(err, release.ErrBusy):
		return reasonBusy
	}
	return reasonFailed
}

func main() {
	pid := flag.Int("pid", 0, "the Poros process to wait for")
	archive := flag.String("archive", "", "the release archive")
	dir := flag.String("dir", "", "the program folder")
	flag.Parse()
	if *archive == "" || *dir == "" {
		os.Exit(2)
	}

	waitForExit(*pid)
	// A failed install still starts Poros: the old files are back in place, and the user must not be left without it.
	if err := release.InstallFile(*archive, *dir, lockWait); err != nil {
		report := reasonOf(err) + "\n" + err.Error()
		_ = os.WriteFile(filepath.Join(filepath.Dir(*archive), errorFile), []byte(report), 0o644)
	} else {
		_ = os.Remove(*archive)
	}

	start := exec.Command(filepath.Join(*dir, release.Program), flag.Args()...)
	start.Dir = *dir
	if err := start.Start(); err != nil {
		os.Exit(1)
	}
}

func waitForExit(pid int) {
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_, _ = process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(exitWait):
	}
}
