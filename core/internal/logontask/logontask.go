// Package logontask gives the core administrator rights at sign-in without a UAC prompt, when the user asked for
// that: a task of the Task Scheduler starts the core with the highest rights as the user signs in, and the shell
// that autostart starts in the same sign-in hands it its channel through a file in the data folder. The window
// keeps no rights.
package logontask

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	// ShellFlag marks the autostart command of a shell that waits for the core of the task.
	ShellFlag = "--logon-core"
	// launchFile is where that shell leaves its channel for the core.
	launchFile = "logon-core.json"
	// removalFile asks the core of the task to remove the task: the setting went off in a core without rights.
	removalFile = "logon-task.remove"
)

// Launch is what the shell hands the core of the task: the same as the arguments of a core it starts itself.
type Launch struct {
	Pipe    string `json:"pipe"`
	Token   string `json:"token"`
	HostPID int    `json:"hostPid"`
	Version string `json:"version"`
}

// WaitLaunch waits for the shell's file, takes it and removes it.
func WaitLaunch(dataDir string, timeout time.Duration) (Launch, error) {
	path := filepath.Join(dataDir, launchFile)
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			_ = os.Remove(path)
			var launch Launch
			if err = json.Unmarshal(data, &launch); err != nil {
				return Launch{}, err
			}
			if launch.Pipe == "" || launch.Token == "" || launch.HostPID == 0 {
				return Launch{}, errors.New("the shell's file lacks its channel")
			}
			return launch, nil
		}
		if time.Now().After(deadline) {
			return Launch{}, errors.New("no shell came")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// MarkRemoval asks the next core of the task to remove the task.
func MarkRemoval(dataDir string) error {
	return os.WriteFile(filepath.Join(dataDir, removalFile), nil, 0o644)
}

// RemovalMarked tells whether the setting went off since the task was made.
func RemovalMarked(dataDir string) bool {
	_, err := os.Stat(filepath.Join(dataDir, removalFile))
	return err == nil
}

func clearRemoval(dataDir string) {
	_ = os.Remove(filepath.Join(dataDir, removalFile))
}
