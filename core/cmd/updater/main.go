// The updater of the portable build: replaces the program files next to it with a release archive, then starts Poros
// again. Poros runs a copy named updater.old, so the archive can replace updater.exe as well.
package main

import (
	"archive/zip"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	program   = "Poros.exe"
	stageName = "update-stage"
	// Poros reads this next to the archive on its next start and tells the user.
	errorFile = "install-error.txt"
	exitWait  = 30 * time.Second
	// PorosCore.exe outlives Poros for a moment, and a virus scanner holds a new file for a while.
	lockWait = 30 * time.Second
)

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
	if err := install(*archive, *dir); err != nil {
		_ = os.WriteFile(filepath.Join(filepath.Dir(*archive), errorFile), []byte(err.Error()), 0o644)
	} else {
		_ = os.Remove(*archive)
	}

	start := exec.Command(filepath.Join(*dir, program), flag.Args()...)
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

func install(archive, dir string) error {
	stage := filepath.Join(dir, stageName)
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	names, err := unpack(archive, stage)
	if err != nil {
		return err
	}
	return replace(dir, stage, names, lockWait)
}

// unpack checks the whole archive before it writes anything.
func unpack(archive, stage string) ([]string, error) {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var names []string
	for _, file := range reader.File {
		// A release is flat, so nothing in it can reach the data folder.
		if file.FileInfo().IsDir() || strings.ContainsAny(file.Name, `/\:`) || file.Name == ".." {
			return nil, fmt.Errorf("unexpected entry %q in the archive", file.Name)
		}
		names = append(names, file.Name)
	}
	if !slices.Contains(names, program) {
		return nil, fmt.Errorf("the archive has no %s", program)
	}

	if err := os.MkdirAll(stage, 0o755); err != nil {
		return nil, err
	}
	for _, file := range reader.File {
		if err := extract(file, filepath.Join(stage, file.Name)); err != nil {
			return nil, err
		}
	}
	return names, nil
}

func extract(file *zip.File, path string) error {
	source, err := file.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err = io.Copy(target, source); err != nil {
		target.Close()
		return err
	}
	return target.Close()
}

// replace swaps the files one by one and puts every old file back if one of them cannot be swapped.
func replace(dir, stage string, names []string, wait time.Duration) error {
	var swapped []string
	rollback := func() {
		for _, name := range slices.Backward(swapped) {
			target := filepath.Join(dir, name)
			_ = os.Remove(target)
			_ = os.Rename(target+".previous", target)
		}
	}
	for _, name := range names {
		target := filepath.Join(dir, name)
		backup := target + ".previous"
		_ = os.Remove(backup)
		err := retry(wait, func() error {
			if err := os.Rename(target, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		})
		if err == nil {
			swapped = append(swapped, name)
			err = retry(wait, func() error { return os.Rename(filepath.Join(stage, name), target) })
		}
		if err != nil {
			rollback()
			return fmt.Errorf("could not replace %s: %w", name, err)
		}
	}
	for _, name := range names {
		// A file that still runs cannot go yet; the next update removes it.
		_ = os.Remove(filepath.Join(dir, name) + ".previous")
	}
	return nil
}

func retry(wait time.Duration, step func() error) error {
	deadline := time.Now().Add(wait)
	for {
		err := step()
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(250 * time.Millisecond)
	}
}
