package wincom

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	clsidShellLink = windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidShellLink   = windows.GUID{Data1: 0x000214f9, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidPersistFile = windows.GUID{Data1: 0x0000010b, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
)

// The places of the methods in the tables of IShellLinkW and IPersistFile.
const (
	linkGetPath             = 3
	linkGetWorkingDirectory = 8
	linkSetWorkingDirectory = 9
	linkSetPath             = 20
	persistLoad             = 5
	persistSave             = 6
	stgmRead                = 0
)

// Shortcut is what a shortcut file starts: the program and its working folder.
type Shortcut struct {
	Target string
	Dir    string
}

// CreateShortcut writes the shortcut link that starts target in dir, over an older one.
func CreateShortcut(link, target, dir string) error {
	shellLink, err := Create(&clsidShellLink, &iidShellLink)
	if err != nil {
		return err
	}
	defer shellLink.Release()
	if err := shellLink.Call(linkSetPath, uintptr(unsafe.Pointer(Wide(target)))); err != nil {
		return err
	}
	if err := shellLink.Call(linkSetWorkingDirectory, uintptr(unsafe.Pointer(Wide(dir)))); err != nil {
		return err
	}
	file, err := shellLink.QueryInterface(&iidPersistFile)
	if err != nil {
		return err
	}
	defer file.Release()
	return file.Call(persistSave, uintptr(unsafe.Pointer(Wide(link))), 1)
}

// ReadShortcut tells what the shortcut link starts. The program need not exist any more.
func ReadShortcut(link string) (Shortcut, error) {
	shellLink, err := Create(&clsidShellLink, &iidShellLink)
	if err != nil {
		return Shortcut{}, err
	}
	defer shellLink.Release()
	file, err := shellLink.QueryInterface(&iidPersistFile)
	if err != nil {
		return Shortcut{}, err
	}
	defer file.Release()
	if err := file.Call(persistLoad, uintptr(unsafe.Pointer(Wide(link))), stgmRead); err != nil {
		return Shortcut{}, err
	}
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	if err := shellLink.Call(linkGetPath, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0, 0); err != nil {
		return Shortcut{}, err
	}
	target := windows.UTF16ToString(buffer)
	if err := shellLink.Call(linkGetWorkingDirectory, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer))); err != nil {
		return Shortcut{}, err
	}
	return Shortcut{Target: target, Dir: windows.UTF16ToString(buffer)}, nil
}
