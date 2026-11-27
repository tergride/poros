//go:build windows

package main

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procCoCreateInstance            = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
	procSHCreateItemFromParsingName = windows.NewLazySystemDLL("shell32.dll").NewProc("SHCreateItemFromParsingName")
)

var (
	clsidFileOpenDialog = windows.GUID{Data1: 0xdc1c5a9c, Data2: 0xe88a, Data3: 0x4dde, Data4: [8]byte{0xa5, 0xa1, 0x60, 0xf8, 0x2a, 0x20, 0xae, 0xf7}}
	iidFileOpenDialog   = windows.GUID{Data1: 0xd57c7288, Data2: 0xd4ad, Data3: 0x4768, Data4: [8]byte{0xbe, 0x02, 0x9d, 0x96, 0x95, 0x32, 0xd9, 0x60}}
	iidShellItem        = windows.GUID{Data1: 0x43826d1e, Data2: 0xe718, Data3: 0x42ee, Data4: [8]byte{0xbc, 0x55, 0xa1, 0xe2, 0x61, 0xc3, 0x7b, 0xfe}}
	clsidShellLink      = windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidShellLink        = windows.GUID{Data1: 0x000214f9, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidPersistFile      = windows.GUID{Data1: 0x0000010b, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
)

// The places of the methods in the tables of IUnknown, IFileDialog, IShellItem, IShellLinkW and IPersistFile.
const (
	unknownQueryInterface   = 0
	unknownRelease          = 2
	dialogShow              = 3
	dialogSetOptions        = 9
	dialogGetOptions        = 10
	dialogSetFolder         = 12
	dialogSetTitle          = 17
	dialogGetResult         = 20
	itemGetDisplayName      = 5
	linkSetWorkingDirectory = 9
	linkSetPath             = 20
	persistSave             = 6
)

const (
	clsctxInprocServer = 0x1
	fosPickFolders     = 0x20
	fosForceFileSystem = 0x40
	sigdnFileSysPath   = 0x80058000
)

// errCancelled is what the folder picker returns when the user closes it.
var errCancelled = syscall.Errno(0x800704c7)

// comObject is an interface pointer of COM: its first field points at the table of its methods.
type comObject struct{ methods *[32]uintptr }

//go:uintptrescapes
func (o *comObject) call(method int, args ...uintptr) error {
	hr, _, _ := syscall.SyscallN(o.methods[method], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return failure(hr)
}

func (o *comObject) release() {
	syscall.SyscallN(o.methods[unknownRelease], uintptr(unsafe.Pointer(o)))
}

func failure(hr uintptr) error {
	if int32(hr) < 0 {
		return syscall.Errno(uint32(hr))
	}
	return nil
}

func wide(text string) *uint16 {
	pointer, _ := windows.UTF16PtrFromString(text)
	return pointer
}

func createObject(class, iid *windows.GUID) (*comObject, error) {
	var object *comObject
	hr, _, _ := syscall.SyscallN(procCoCreateInstance.Addr(), uintptr(unsafe.Pointer(class)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&object)))
	if err := failure(hr); err != nil {
		return nil, err
	}
	return object, nil
}

func shellItem(path string) (*comObject, error) {
	var item *comObject
	hr, _, _ := syscall.SyscallN(procSHCreateItemFromParsingName.Addr(), uintptr(unsafe.Pointer(wide(path))), 0,
		uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
	if err := failure(hr); err != nil {
		return nil, err
	}
	return item, nil
}

// pickFolder shows the folder picker of Windows over owner, open at start when it is not empty. It returns "" when
// the user cancels.
func pickFolder(owner uintptr, title, start string) (string, error) {
	dialog, err := createObject(&clsidFileOpenDialog, &iidFileOpenDialog)
	if err != nil {
		return "", err
	}
	defer dialog.release()

	var options uint32
	if err := dialog.call(dialogGetOptions, uintptr(unsafe.Pointer(&options))); err != nil {
		return "", err
	}
	if err := dialog.call(dialogSetOptions, uintptr(options|fosPickFolders|fosForceFileSystem)); err != nil {
		return "", err
	}
	_ = dialog.call(dialogSetTitle, uintptr(unsafe.Pointer(wide(title))))
	if start != "" {
		if folder, err := shellItem(start); err == nil {
			_ = dialog.call(dialogSetFolder, uintptr(unsafe.Pointer(folder)))
			folder.release()
		}
	}
	if err := dialog.call(dialogShow, owner); errors.Is(err, errCancelled) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	var item *comObject
	if err := dialog.call(dialogGetResult, uintptr(unsafe.Pointer(&item))); err != nil {
		return "", err
	}
	defer item.release()
	var name *uint16
	if err := item.call(itemGetDisplayName, sigdnFileSysPath, uintptr(unsafe.Pointer(&name))); err != nil {
		return "", err
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(name))
	return windows.UTF16PtrToString(name), nil
}

// createShortcut writes the shortcut link that starts target in dir, over an older one.
func createShortcut(link, target, dir string) error {
	shellLink, err := createObject(&clsidShellLink, &iidShellLink)
	if err != nil {
		return err
	}
	defer shellLink.release()
	if err := shellLink.call(linkSetPath, uintptr(unsafe.Pointer(wide(target)))); err != nil {
		return err
	}
	if err := shellLink.call(linkSetWorkingDirectory, uintptr(unsafe.Pointer(wide(dir)))); err != nil {
		return err
	}
	var file *comObject
	if err := shellLink.call(unknownQueryInterface, uintptr(unsafe.Pointer(&iidPersistFile)), uintptr(unsafe.Pointer(&file))); err != nil {
		return err
	}
	defer file.release()
	return file.call(persistSave, uintptr(unsafe.Pointer(wide(link))), 1)
}
