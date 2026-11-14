//go:build windows

package marker

import (
	"errors"

	"golang.org/x/sys/windows"
)

// Marker is a mutex in the global namespace of Windows. Its default access list keeps other users out, so they find
// it taken by ERROR_ACCESS_DENIED rather than by opening it.
type Marker struct {
	handle windows.Handle
}

func objectName(name string) (*uint16, error) {
	return windows.UTF16PtrFromString(`Global\` + Namespace + "-" + name)
}

// Hold takes the marker until Release, or fails with ErrTaken when another process holds it.
func Hold(name string) (*Marker, error) {
	object, err := objectName(name)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateMutex(nil, false, object)
	switch {
	case errors.Is(err, windows.ERROR_ALREADY_EXISTS):
		_ = windows.CloseHandle(handle)
		return nil, ErrTaken
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return nil, ErrTaken
	case err != nil:
		return nil, err
	}
	return &Marker{handle: handle}, nil
}

// Release lets go of the marker; a nil marker is nothing to let go of.
func (m *Marker) Release() {
	if m == nil || m.handle == 0 {
		return
	}
	_ = windows.CloseHandle(m.handle)
	m.handle = 0
}

// Taken tells whether any process holds the marker, this one included: the caller knows what it holds itself.
func Taken(name string) bool {
	object, err := objectName(name)
	if err != nil {
		return false
	}
	handle, err := windows.OpenMutex(windows.SYNCHRONIZE, false, object)
	if err == nil {
		_ = windows.CloseHandle(handle)
		return true
	}
	return errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
