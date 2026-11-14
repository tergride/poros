package engine

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetProcessHandleCount = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount")

// processHandles counts the handles the process holds, -1 when Windows does not tell.
func processHandles() int {
	var count uint32
	if ok, _, _ := procGetProcessHandleCount.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&count))); ok == 0 {
		return -1
	}
	return int(count)
}
