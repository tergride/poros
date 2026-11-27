//go:build !windows

package engine

import "errors"

// runningPrograms waits for the processes of Linux and Android, which come with their stages.
func runningPrograms() ([]RunningProgram, error) {
	return nil, errors.New("the running programs are not listed on this system yet")
}
