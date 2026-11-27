package engine

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// runningPrograms takes a snapshot of the processes. A process that does not let the core read its path, as one of
// another user to a core without administrator rights, comes by its name alone.
func runningPrograms() ([]RunningProgram, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("taking a snapshot of the processes: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	systemDir, _ := windows.GetSystemWindowsDirectory()
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	var found []RunningProgram
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		// Process 0 is the idle time of the processors, no program.
		if entry.ProcessID == 0 {
			continue
		}
		path := imagePath(entry.ProcessID, buf)
		name := windows.UTF16ToString(entry.ExeFile[:])
		if path != "" {
			name = path[strings.LastIndexAny(path, `\/`)+1:]
		}
		found = append(found, RunningProgram{Name: name, Path: path})
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("reading the snapshot of the processes: %w", err)
	}
	return distinctPrograms(found, systemDir), nil
}

func imagePath(pid uint32, buf []uint16) string {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(process)
	size := uint32(len(buf))
	if windows.QueryFullProcessImageName(process, 0, &buf[0], &size) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// distinctPrograms keeps each program once, its file named in any case as Windows does: a program by its path, and
// one without a path by its name, unless a program of that name has a path. A program in the folder of Windows, or
// without a path, is the system's.
func distinctPrograms(found []RunningProgram, systemDir string) []RunningProgram {
	if systemDir != "" {
		systemDir = strings.ToLower(strings.TrimRight(systemDir, `\`)) + `\`
	}
	withPath := map[string]bool{}
	for _, p := range found {
		if p.Path != "" {
			withPath[strings.ToLower(p.Name)] = true
		}
	}
	seen := map[string]bool{}
	var programs []RunningProgram
	for _, p := range found {
		key := strings.ToLower(p.Path)
		if p.Path == "" {
			if withPath[strings.ToLower(p.Name)] {
				continue
			}
			// No path has a colon at the start: a name cannot be taken for a path.
			key = ":" + strings.ToLower(p.Name)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		p.System = p.Path == "" || (systemDir != "" && strings.HasPrefix(key, systemDir))
		programs = append(programs, p)
	}
	return programs
}
