// Package wincom calls the COM objects of Windows that Poros needs without a COM library: shortcuts, the folder
// picker of the archive's window and the list of networks. A caller keeps COM initialized on a locked thread.
package wincom

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procCoCreateInstance = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
	procSysFreeString    = windows.NewLazySystemDLL("oleaut32.dll").NewProc("SysFreeString")
)

const (
	clsctxInprocServer = 0x1
	clsctxLocalServer  = 0x4

	unknownQueryInterface = 0
	unknownRelease        = 2
)

// Object is an interface pointer of COM: its first field points at the table of its methods.
type Object struct{ methods *[32]uintptr }

// Call calls the method at its place in the table, the object itself first.
//
//go:uintptrescapes
func (o *Object) Call(method int, args ...uintptr) error {
	hr, _, _ := syscall.SyscallN(o.methods[method], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return Failure(hr)
}

// QueryInterface gives the object's interface iid.
func (o *Object) QueryInterface(iid *windows.GUID) (*Object, error) {
	var other *Object
	if err := o.Call(unknownQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&other))); err != nil {
		return nil, err
	}
	return other, nil
}

func (o *Object) Release() {
	syscall.SyscallN(o.methods[unknownRelease], uintptr(unsafe.Pointer(o)))
}

// Failure is the error of an HRESULT that failed, nil for one that worked.
func Failure(hr uintptr) error {
	if int32(hr) < 0 {
		return syscall.Errno(uint32(hr))
	}
	return nil
}

// Create makes the object of class with its interface iid, in this process.
func Create(class, iid *windows.GUID) (*Object, error) {
	return create(class, iid, clsctxInprocServer)
}

func create(class, iid *windows.GUID, context uintptr) (*Object, error) {
	var object *Object
	hr, _, _ := syscall.SyscallN(procCoCreateInstance.Addr(), uintptr(unsafe.Pointer(class)), 0, context,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&object)))
	if err := Failure(hr); err != nil {
		return nil, err
	}
	return object, nil
}

// Wide is text as a string of Windows that ends in zero.
func Wide(text string) *uint16 {
	pointer, _ := windows.UTF16PtrFromString(text)
	return pointer
}

// takeString reads a BSTR a method gave and frees it.
func takeString(text *uint16) string {
	if text == nil {
		return ""
	}
	defer syscall.SyscallN(procSysFreeString.Addr(), uintptr(unsafe.Pointer(text)))
	return windows.UTF16PtrToString(text)
}
