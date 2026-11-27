package engine

import (
	"cmp"
	"slices"
	"strings"
)

// RunningPrograms gives the programs running now: those of the user first, then those of the system, each part by
// name and then by path.
func RunningPrograms() ([]RunningProgram, error) {
	programs, err := runningPrograms()
	if err != nil {
		return nil, err
	}
	slices.SortFunc(programs, func(a, b RunningProgram) int {
		if a.System != b.System {
			if a.System {
				return 1
			}
			return -1
		}
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.Path, b.Path))
	})
	return nonNil(programs), nil
}
