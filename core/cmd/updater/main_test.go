package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeArchive(t *testing.T, path string, files map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	for name, text := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

func programFolder(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, program), "old program")
	writeFile(t, filepath.Join(dir, "PorosCore.exe"), "old core")
	writeFile(t, filepath.Join(dir, "config", "poros.db"), "the user's data")
	return dir
}

func assertUntouched(t *testing.T, dir string) {
	t.Helper()
	if got := readFile(t, filepath.Join(dir, program)); got != "old program" {
		t.Fatalf("program is %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "PorosCore.exe")); got != "old core" {
		t.Fatalf("core is %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, stageName)); !os.IsNotExist(err) {
		t.Fatalf("the stage folder stayed: %v", err)
	}
}

func TestInstallReplacesTheProgramAndLeavesTheData(t *testing.T) {
	dir := programFolder(t)
	archive := filepath.Join(t.TempDir(), "release.zip")
	writeArchive(t, archive, map[string]string{program: "new program", "PorosCore.exe": "new core", "updater.exe": "new updater"})

	if err := install(archive, dir); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]string{program: "new program", "PorosCore.exe": "new core", "updater.exe": "new updater"} {
		if got := readFile(t, filepath.Join(dir, name)); got != want {
			t.Errorf("%s is %q", name, got)
		}
		if _, err := os.Stat(filepath.Join(dir, name+".previous")); !os.IsNotExist(err) {
			t.Errorf("the backup of %s stayed: %v", name, err)
		}
	}
	if got := readFile(t, filepath.Join(dir, "config", "poros.db")); got != "the user's data" {
		t.Errorf("data is %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, stageName)); !os.IsNotExist(err) {
		t.Errorf("the stage folder stayed: %v", err)
	}
}

func TestInstallRefusesAnArchiveThatLeavesTheFolder(t *testing.T) {
	for _, name := range []string{"../outside.exe", `..\outside.exe`, "config/poros.db", `C:\outside.exe`, ".."} {
		t.Run(name, func(t *testing.T) {
			dir := programFolder(t)
			archive := filepath.Join(t.TempDir(), "release.zip")
			writeArchive(t, archive, map[string]string{program: "new program", name: "bad"})

			if err := install(archive, dir); err == nil {
				t.Fatal("the archive was accepted")
			}
			assertUntouched(t, dir)
			if got := readFile(t, filepath.Join(dir, "config", "poros.db")); got != "the user's data" {
				t.Fatalf("data is %q", got)
			}
		})
	}
}

func TestInstallRefusesAnArchiveWithoutTheProgram(t *testing.T) {
	dir := programFolder(t)
	archive := filepath.Join(t.TempDir(), "release.zip")
	writeArchive(t, archive, map[string]string{"PorosCore.exe": "new core"})

	if err := install(archive, dir); err == nil {
		t.Fatal("the archive was accepted")
	}
	assertUntouched(t, dir)
}

func TestInstallKeepsTheProgramWhenTheArchiveIsDamaged(t *testing.T) {
	dir := programFolder(t)
	archive := filepath.Join(t.TempDir(), "release.zip")
	writeFile(t, archive, "not a zip file")

	if err := install(archive, dir); err == nil {
		t.Fatal("the archive was accepted")
	}
	assertUntouched(t, dir)
}

func TestReplacePutsEverythingBackWhenOneFileFails(t *testing.T) {
	dir := programFolder(t)
	stage := t.TempDir()
	writeFile(t, filepath.Join(stage, program), "new program")

	// The second file has nothing staged, so its swap fails after the first one went through.
	err := replace(dir, stage, []string{program, "PorosCore.exe"}, 0)
	if err == nil {
		t.Fatal("the swap succeeded")
	}
	if got := readFile(t, filepath.Join(dir, program)); got != "old program" {
		t.Fatalf("program is %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "PorosCore.exe")); got != "old core" {
		t.Fatalf("core is %q", got)
	}
}
