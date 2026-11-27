// Package release puts the files of a release archive into a program folder: the updater replaces the program with a
// new release, the self-extracting archive unpacks the first one.
package release

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Program is the file every release archive has.
const Program = "Poros.exe"

const stageName = "update-stage"

var (
	// ErrDamaged is an archive that is no release of Poros.
	ErrDamaged = errors.New("the archive is no release of Poros")
	// ErrBusy is a program file that could not be replaced, most likely held by another program.
	ErrBusy = errors.New("a program file could not be replaced")
)

// failure keeps the message of its cause, and errors.Is finds its kind.
type failure struct{ kind, cause error }

func (f *failure) Error() string   { return f.cause.Error() }
func (f *failure) Unwrap() []error { return []error{f.kind, f.cause} }

// InstallFile installs the archive at path, see Install.
func InstallFile(path, dir string, wait time.Duration) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return &failure{ErrDamaged, err}
	}
	defer reader.Close()
	return Install(&reader.Reader, dir, wait)
}

// Install puts the files of archive into dir and creates dir when it is missing. It checks and unpacks the whole
// archive before it touches a file of dir, and puts every old file back when one of them cannot be replaced within
// wait. Everything else in dir, the data folder too, stays.
func Install(archive *zip.Reader, dir string, wait time.Duration) error {
	stage := filepath.Join(dir, stageName)
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	names, err := unpack(archive, stage)
	if err != nil {
		return &failure{ErrDamaged, err}
	}
	if err = replace(dir, stage, names, wait); err != nil {
		return &failure{ErrBusy, err}
	}
	return nil
}

func unpack(archive *zip.Reader, stage string) ([]string, error) {
	var names []string
	for _, file := range archive.File {
		// A release is flat, so nothing in it can reach the data folder.
		if file.FileInfo().IsDir() || strings.ContainsAny(file.Name, `/\:`) || file.Name == ".." {
			return nil, fmt.Errorf("unexpected entry %q in the archive", file.Name)
		}
		names = append(names, file.Name)
	}
	if !slices.Contains(names, Program) {
		return nil, fmt.Errorf("the archive has no %s", Program)
	}

	if err := os.MkdirAll(stage, 0o755); err != nil {
		return nil, err
	}
	for _, file := range archive.File {
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
