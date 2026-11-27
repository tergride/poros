package engine

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// A file comes once whatever the case of its path, a process without a path only when no program of its name has
// one, and only the folder of Windows itself makes a program the system's.
func TestDistinctProgramsNameFilesAsWindows(t *testing.T) {
	telegram := `C:\Users\Me\AppData\Roaming\Telegram Desktop\Telegram.exe`
	found := []RunningProgram{
		{Name: "Telegram.exe", Path: telegram},
		{Name: "telegram.exe", Path: strings.ToLower(telegram)},
		{Name: "python.exe", Path: `C:\Python\python.exe`},
		{Name: "python.exe", Path: `D:\venv\Scripts\python.exe`},
		{Name: "svchost.exe", Path: `C:\WINDOWS\System32\svchost.exe`},
		{Name: "SVCHOST.EXE"},
		{Name: "csrss.exe"},
		{Name: "csrss.exe"},
		{Name: "app.exe", Path: `C:\WindowsApps\app.exe`},
	}
	want := []RunningProgram{
		{Name: "Telegram.exe", Path: telegram},
		{Name: "python.exe", Path: `C:\Python\python.exe`},
		{Name: "python.exe", Path: `D:\venv\Scripts\python.exe`},
		{Name: "svchost.exe", Path: `C:\WINDOWS\System32\svchost.exe`, System: true},
		{Name: "csrss.exe", System: true},
		{Name: "app.exe", Path: `C:\WindowsApps\app.exe`},
	}
	if got := distinctPrograms(found, `C:\Windows`); !slices.Equal(got, want) {
		t.Errorf("the programs are\n%+v\nnot\n%+v", got, want)
	}
}

// The snapshot finds the test itself as a program of the user's, which comes before those of the system.
func TestRunningProgramsFindTheTest(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	programs, err := RunningPrograms()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(programs, func(p RunningProgram) bool { return strings.EqualFold(p.Path, self) })
	if i < 0 {
		t.Fatalf("%s is not among the %d running programs", self, len(programs))
	}
	if p := programs[i]; p.System || !strings.EqualFold(p.Name, self[strings.LastIndex(self, `\`)+1:]) {
		t.Errorf("the test runs as %+v", p)
	}
	first := slices.IndexFunc(programs, func(p RunningProgram) bool { return p.System })
	if first < 0 {
		t.Fatal("no program of the system is running")
	}
	if slices.ContainsFunc(programs[first:], func(p RunningProgram) bool { return !p.System }) {
		t.Error("a program of the user's comes after those of the system")
	}
}
