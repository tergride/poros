// Package logontask removes the task of the Task Scheduler that earlier versions of Poros made to give the core
// administrator rights at sign-in. The task starts the core with --logon at the next sign-in, and that core, which has
// the rights, removes it; the cleanup removes it too.
package logontask

import (
	"os"
	"path/filepath"
)

// removalFile in the data folder tells that a task of the folder may be there although a user without rights does
// not see it.
const removalFile = "logon-task.remove"

// MarkRemoval marks the task of the folder for removal.
func MarkRemoval(dataDir string) error {
	return os.WriteFile(filepath.Join(dataDir, removalFile), nil, 0o644)
}

// RemovalMarked tells whether a task of the folder may still be there.
func RemovalMarked(dataDir string) bool {
	_, err := os.Stat(filepath.Join(dataDir, removalFile))
	return err == nil
}

func clearRemoval(dataDir string) {
	_ = os.Remove(filepath.Join(dataDir, removalFile))
}
