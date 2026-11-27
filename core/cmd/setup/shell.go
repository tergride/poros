//go:build windows

package main

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"PorosCore/internal/wincom"
)

var procSHCreateItemFromParsingName = windows.NewLazySystemDLL("shell32.dll").NewProc("SHCreateItemFromParsingName")

var (
	clsidFileOpenDialog = windows.GUID{Data1: 0xdc1c5a9c, Data2: 0xe88a, Data3: 0x4dde, Data4: [8]byte{0xa5, 0xa1, 0x60, 0xf8, 0x2a, 0x20, 0xae, 0xf7}}
	iidFileOpenDialog   = windows.GUID{Data1: 0xd57c7288, Data2: 0xd4ad, Data3: 0x4768, Data4: [8]byte{0xbe, 0x02, 0x9d, 0x96, 0x95, 0x32, 0xd9, 0x60}}
	iidShellItem        = windows.GUID{Data1: 0x43826d1e, Data2: 0xe718, Data3: 0x42ee, Data4: [8]byte{0xbc, 0x55, 0xa1, 0xe2, 0x61, 0xc3, 0x7b, 0xfe}}
)

// The places of the methods in the tables of IFileDialog and IShellItem.
const (
	dialogShow         = 3
	dialogSetOptions   = 9
	dialogGetOptions   = 10
	dialogSetFolder    = 12
	dialogSetTitle     = 17
	dialogGetResult    = 20
	itemGetDisplayName = 5
)

const (
	fosPickFolders     = 0x20
	fosForceFileSystem = 0x40
	sigdnFileSysPath   = 0x80058000
)

// errCancelled is what the folder picker returns when the user closes it.
var errCancelled = syscall.Errno(0x800704c7)

func wide(text string) *uint16 {
	return wincom.Wide(text)
}

func shellItem(path string) (*wincom.Object, error) {
	var item *wincom.Object
	hr, _, _ := syscall.SyscallN(procSHCreateItemFromParsingName.Addr(), uintptr(unsafe.Pointer(wide(path))), 0,
		uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
	if err := wincom.Failure(hr); err != nil {
		return nil, err
	}
	return item, nil
}

// pickFolder shows the folder picker of Windows over owner, open at start when it is not empty. It returns "" when
// the user cancels.
func pickFolder(owner uintptr, title, start string) (string, error) {
	dialog, err := wincom.Create(&clsidFileOpenDialog, &iidFileOpenDialog)
	if err != nil {
		return "", err
	}
	defer dialog.Release()

	var options uint32
	if err := dialog.Call(dialogGetOptions, uintptr(unsafe.Pointer(&options))); err != nil {
		return "", err
	}
	if err := dialog.Call(dialogSetOptions, uintptr(options|fosPickFolders|fosForceFileSystem)); err != nil {
		return "", err
	}
	_ = dialog.Call(dialogSetTitle, uintptr(unsafe.Pointer(wide(title))))
	if start != "" {
		if folder, err := shellItem(start); err == nil {
			_ = dialog.Call(dialogSetFolder, uintptr(unsafe.Pointer(folder)))
			folder.Release()
		}
	}
	if err := dialog.Call(dialogShow, owner); errors.Is(err, errCancelled) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	var item *wincom.Object
	if err := dialog.Call(dialogGetResult, uintptr(unsafe.Pointer(&item))); err != nil {
		return "", err
	}
	defer item.Release()
	var name *uint16
	if err := item.Call(itemGetDisplayName, sigdnFileSysPath, uintptr(unsafe.Pointer(&name))); err != nil {
		return "", err
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(name))
	return windows.UTF16PtrToString(name), nil
}
