//go:build windows

// The self-extracting archive of Poros: a window that unpacks the release archive at the end of this file into a
// folder, then adds shortcuts on the desktop and in the Start menu and starts Poros if asked. It writes nothing to the
// registry, so deleting the folder and the shortcuts removes Poros.
package main

import (
	"archive/zip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"PorosCore/internal/release"
)

// version is set by the release build.
var version string

// The release archive comes after the program in the same file, then a trailer: the size of the archive as eight
// bytes little-endian and a mark. The release build appends both.
const (
	trailerMark = "PorosSFX"
	trailerSize = int64(8 + len(trailerMark))
)

// A virus scanner holds a new file for a while.
const lockWait = 10 * time.Second

var errNoArchive = errors.New("the file has no release archive at its end")

// The window and COM stay on the main thread.
func init() { runtime.LockOSThread() }

func main() {
	text := textsFor(russian())
	path, err := os.Executable()
	var file *os.File
	var archive *zip.Reader
	if err == nil {
		file, archive, err = openArchive(path)
	}
	if err != nil {
		showMessage(0, text.noArchive, windows.MB_ICONERROR)
		os.Exit(1)
	}
	defer file.Close()

	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil {
		defer windows.CoUninitialize()
	}
	if err := showWindow(archive, text); err != nil {
		showMessage(0, fmt.Sprintf(text.failed, err), windows.MB_ICONERROR)
	}
}

// openArchive reads the release archive at the end of the file at path.
func openArchive(path string) (*os.File, *zip.Reader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	archive, err := archiveOf(file)
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return file, archive, nil
}

func archiveOf(file *os.File) (*zip.Reader, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size < trailerSize {
		return nil, errNoArchive
	}
	trailer := make([]byte, trailerSize)
	if _, err := file.ReadAt(trailer, size-trailerSize); err != nil {
		return nil, err
	}
	length := binary.LittleEndian.Uint64(trailer)
	if string(trailer[8:]) != trailerMark || length == 0 || length > uint64(size-trailerSize) {
		return nil, errNoArchive
	}
	start := size - trailerSize - int64(length)
	return zip.NewReader(io.NewSectionReader(file, start, int64(length)), int64(length))
}

// defaultFolder is in the programs folder of this user, which needs no administrator rights.
func defaultFolder() string {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return ""
	}
	return filepath.Join(local, "Programs", "Poros")
}

// pickedFolder is where Poros goes when the user picks folder: a folder of its own inside, unless folder already
// holds Poros or is an empty folder named Poros. A folder named so with other files in it, such as the profile of a
// Windows user named poros, gets one inside too.
func pickedFolder(folder string) string {
	if _, err := os.Stat(filepath.Join(folder, release.Program)); err == nil {
		return folder
	}
	if strings.EqualFold(filepath.Base(folder), "Poros") {
		if entries, err := os.ReadDir(folder); err == nil && len(entries) == 0 {
			return folder
		}
	}
	return filepath.Join(folder, "Poros")
}

// pickerStart is the folder the picker opens: the nearest of folder and its parents that exists.
func pickerStart(folder string) string {
	for folder != "" {
		if info, err := os.Stat(folder); err == nil && info.IsDir() {
			return folder
		}
		parent := filepath.Dir(folder)
		if parent == folder {
			break
		}
		folder = parent
	}
	return ""
}

// running tells whether Poros runs from dir: Windows does not open the file of a running program for writing.
func running(dir string) bool {
	for _, name := range []string{release.Program, "PorosCore.exe"} {
		file, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY, 0)
		if err == nil {
			file.Close()
		} else if errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return true
		}
	}
	return false
}

// writable creates dir when it is missing and checks that a file can be made in it.
func writable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	probe, err := os.CreateTemp(dir, ".write-check-*")
	if err != nil {
		return err
	}
	probe.Close()
	return os.Remove(probe.Name())
}
