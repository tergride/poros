package qr

import (
	"fmt"
	"image"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")

	getSystemMetrics             = user32.NewProc("GetSystemMetrics")
	getDC                        = user32.NewProc("GetDC")
	releaseDC                    = user32.NewProc("ReleaseDC")
	setThreadDpiAwarenessContext = user32.NewProc("SetThreadDpiAwarenessContext")
	createCompatibleDC           = gdi32.NewProc("CreateCompatibleDC")
	createCompatibleBitmap       = gdi32.NewProc("CreateCompatibleBitmap")
	selectObject                 = gdi32.NewProc("SelectObject")
	bitBlt                       = gdi32.NewProc("BitBlt")
	getDIBits                    = gdi32.NewProc("GetDIBits")
	deleteObject                 = gdi32.NewProc("DeleteObject")
	deleteDC                     = gdi32.NewProc("DeleteDC")
)

const (
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79
	srcCopy           = 0x00CC0020
	captureBlt        = 0x40000000
	// dpiPerMonitorAwareV2 is DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2: the snapshot in real pixels, not scaled
	// down for a program that does not know the scale of the screen.
	dpiPerMonitorAwareV2 = ^uintptr(3)
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// Screen takes a snapshot of all monitors at once, in grey: a QR code needs no colors. A window that keeps itself out
// of snapshots, as Poros does while it scans, is not on it.
func Screen() (*image.Gray, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Windows before 10 1607 has no such call: the snapshot is then as the system scales it.
	if setThreadDpiAwarenessContext.Find() == nil {
		previous, _, _ := setThreadDpiAwarenessContext.Call(dpiPerMonitorAwareV2)
		if previous != 0 {
			defer setThreadDpiAwarenessContext.Call(previous)
		}
	}
	metric := func(index uintptr) int32 {
		value, _, _ := getSystemMetrics.Call(index)
		return int32(value)
	}
	x, y := metric(smXVirtualScreen), metric(smYVirtualScreen)
	width, height := metric(smCXVirtualScreen), metric(smCYVirtualScreen)
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("the screen has no size")
	}

	screen, _, err := getDC.Call(0)
	if screen == 0 {
		return nil, fmt.Errorf("GetDC: %w", err)
	}
	defer releaseDC.Call(0, screen)
	memory, _, err := createCompatibleDC.Call(screen)
	if memory == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC: %w", err)
	}
	defer deleteDC.Call(memory)
	bitmap, _, err := createCompatibleBitmap.Call(screen, uintptr(width), uintptr(height))
	if bitmap == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap: %w", err)
	}
	defer deleteObject.Call(bitmap)

	old, _, _ := selectObject.Call(memory, bitmap)
	copied, _, err := bitBlt.Call(memory, 0, 0, uintptr(width), uintptr(height), screen, uintptr(x), uintptr(y), srcCopy|captureBlt)
	// GetDIBits wants the bitmap out of the device context.
	selectObject.Call(memory, old)
	if copied == 0 {
		return nil, fmt.Errorf("BitBlt: %w", err)
	}

	// 32 bits a pixel, top row first: blue, green, red and an unused byte.
	header := bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: width, Height: -height, Planes: 1, BitCount: 32}
	var info struct {
		header bitmapInfoHeader
		colors [4]byte
	}
	info.header = header
	pixels := make([]byte, int(width)*int(height)*4)
	lines, _, err := getDIBits.Call(memory, bitmap, 0, uintptr(height), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&info)), 0)
	if lines == 0 {
		return nil, fmt.Errorf("GetDIBits: %w", err)
	}
	gray := image.NewGray(image.Rect(0, 0, int(width), int(height)))
	for i := range gray.Pix {
		b, g, r := uint32(pixels[i*4]), uint32(pixels[i*4+1]), uint32(pixels[i*4+2])
		gray.Pix[i] = uint8((r*299 + g*587 + b*114) / 1000)
	}
	return gray, nil
}
