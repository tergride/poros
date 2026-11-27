//go:build windows

package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// writeSetup writes a file as the release build does: the program, the archive and the trailer.
func writeSetup(t *testing.T, program []byte, archive []byte, length uint64, mark string) string {
	t.Helper()
	var file bytes.Buffer
	file.Write(program)
	file.Write(archive)
	file.Write(binary.LittleEndian.AppendUint64(nil, length))
	file.WriteString(mark)
	path := filepath.Join(t.TempDir(), "Poros-setup.exe")
	if err := os.WriteFile(path, file.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, text := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func TestOpenArchiveReadsTheArchiveAfterTheProgram(t *testing.T) {
	archive := zipOf(t, map[string]string{"Poros.exe": "the program", "LICENSE": "the license"})
	path := writeSetup(t, []byte("MZ the setup program"), archive, uint64(len(archive)), trailerMark)

	file, reader, err := openArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if len(reader.File) != 2 {
		t.Fatalf("the archive has %d files", len(reader.File))
	}
	entry, err := reader.Open("Poros.exe")
	if err != nil {
		t.Fatal(err)
	}
	defer entry.Close()
	if text, _ := io.ReadAll(entry); string(text) != "the program" {
		t.Fatalf("Poros.exe is %q", text)
	}
}

func TestOpenArchiveRefusesAFileWithoutOne(t *testing.T) {
	archive := zipOf(t, map[string]string{"Poros.exe": "the program"})
	for name, path := range map[string]string{
		"no trailer":             writeSetup(t, []byte("MZ the setup program"), nil, 0, ""),
		"another mark":           writeSetup(t, []byte("MZ"), archive, uint64(len(archive)), "Another!"),
		"too long":               writeSetup(t, nil, archive, uint64(len(archive))+1, trailerMark),
		"empty archive":          writeSetup(t, []byte("MZ"), nil, 0, trailerMark),
		"shorter than a trailer": writeSetup(t, nil, nil, 0, ""),
	} {
		t.Run(name, func(t *testing.T) {
			if file, _, err := openArchive(path); !errors.Is(err, errNoArchive) {
				if file != nil {
					file.Close()
				}
				t.Fatalf("the file was read: %v", err)
			}
		})
	}
}

func TestPickedFolderKeepsPorosInAFolderOfItsOwn(t *testing.T) {
	folder := func(name string, files ...string) string {
		dir := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if err := os.WriteFile(filepath.Join(dir, file), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	installed := folder("Apps", "Poros.exe")
	empty := folder("poros")
	profile := folder("poros", "NTUSER.DAT")
	for picked, want := range map[string]string{
		`D:\Apps`: `D:\Apps\Poros`,
		`D:\`:     `D:\Poros`,
		installed: installed,
		empty:     empty,
		profile:   filepath.Join(profile, "Poros"),
	} {
		if got := pickedFolder(picked); got != want {
			t.Errorf("%s gives %s", picked, got)
		}
	}
}

func TestRunningSeesAProgramFileInUse(t *testing.T) {
	// Windows refuses to open the file of a running program for writing, the file of this test too.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if file, err := os.OpenFile(self, os.O_WRONLY, 0); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		if file != nil {
			file.Close()
		}
		t.Fatalf("the running test opens for writing: %v", err)
	}

	dir := t.TempDir()
	if running(dir) {
		t.Fatal("an empty folder runs Poros")
	}
	program := filepath.Join(dir, "Poros.exe")
	if err := os.WriteFile(program, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if running(dir) {
		t.Fatal("a closed Poros.exe runs")
	}
	// A handle without shared write access stands in for the running program.
	handle, err := windows.CreateFile(windows.StringToUTF16Ptr(program), windows.GENERIC_READ, windows.FILE_SHARE_READ, nil,
		windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if !running(dir) {
		t.Fatal("Poros.exe in use does not run")
	}
}

const (
	linkGetPath             = 3
	linkGetWorkingDirectory = 8
)

func TestCreateShortcutStartsTheProgramInItsFolder(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		t.Fatal(err)
	}
	defer windows.CoUninitialize()

	dir := filepath.Join(t.TempDir(), "Programs", "Poros")
	program := filepath.Join(dir, "Poros.exe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(program, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), shortcutName)
	// A second shortcut replaces the first.
	for range 2 {
		if err := createShortcut(link, program, dir); err != nil {
			t.Fatal(err)
		}
	}

	shellLink, err := createObject(&clsidShellLink, &iidShellLink)
	if err != nil {
		t.Fatal(err)
	}
	defer shellLink.release()
	var file *comObject
	if err := shellLink.call(unknownQueryInterface, uintptr(unsafe.Pointer(&iidPersistFile)), uintptr(unsafe.Pointer(&file))); err != nil {
		t.Fatal(err)
	}
	defer file.release()
	const persistLoad = 5
	if err := file.call(persistLoad, uintptr(unsafe.Pointer(wide(link))), 0); err != nil {
		t.Fatal(err)
	}
	buffer := make([]uint16, windows.MAX_PATH)
	if err := shellLink.call(linkGetPath, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0, 0); err != nil {
		t.Fatal(err)
	}
	if got := syscall.UTF16ToString(buffer); !strings.EqualFold(got, program) {
		t.Errorf("the shortcut starts %s", got)
	}
	if err := shellLink.call(linkGetWorkingDirectory, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer))); err != nil {
		t.Fatal(err)
	}
	if got := syscall.UTF16ToString(buffer); !strings.EqualFold(got, dir) {
		t.Errorf("the shortcut starts in %s", got)
	}
}
