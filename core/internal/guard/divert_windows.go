package guard

import (
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WinDivert, the driver under zapret and GoodbyeDPI, takes packets and sends them on from the kernel: to Windows
// each is then a raw TCP or UDP connection of System, and the block stopped the core's own that way. While the
// driver is loaded the block lets those out. A program the block stops never gets a packet as far as WinDivert.
// The raw endpoint is what keeps it narrow: what System dials by itself is none, such as SMB and its QUIC on 443,
// which any program provokes with a path to a computer and which would show that computer the real address.

// systemAppID is the app ID of the kernel: a name, not a path.
const systemAppID = "System"

func divertRules(pin *runtime.Pinner) []wfpRule {
	name := windows.StringToUTF16(systemAppID)
	pin.Pin(&name[0])
	appID := &fwpByteBlob{size: uint32(len(name) * 2), data: (*uint8)(unsafe.Pointer(&name[0]))}
	pin.Pin(appID)
	connect := []windows.GUID{layerALEAuthConnectV4, layerALEAuthConnectV6}
	return []wfpRule{
		{"permit WinDivert", connect, weightDivert, fwpActionPermit, []fwpmFilterCondition0{
			condition(conditionALEAppID, fwpMatchEqual, fwpByteBlobType, uintptr(unsafe.Pointer(appID))),
			condition(conditionFlags, fwpMatchFlagsAllSet, fwpUint32, fwpConditionFlagIsRawEndpoint),
			equal8(conditionIPProtocol, ipProtoTCP), equal8(conditionIPProtocol, ipProtoUDP),
		}},
	}
}

// divertLoaded tells whether the driver of WinDivert is in the kernel. It stays there as long as a program has it
// open, and after that until it is stopped.
func divertLoaded() (bool, error) {
	drivers, err := loadedDrivers()
	if err != nil {
		return false, err
	}
	for _, name := range drivers {
		if isDivertDriver(name) {
			return true, nil
		}
	}
	return false, nil
}

// isDivertDriver knows the driver by its file: the names of its service differ from program to program.
func isDivertDriver(name string) bool {
	name = strings.ToLower(name)
	return strings.HasPrefix(name, "windivert") && strings.HasSuffix(name, ".sys")
}

// loadedDrivers are the file names of what is loaded into the kernel.
func loadedDrivers() ([]string, error) {
	size := uint32(128 << 10)
	for {
		buffer := make([]uint64, size/8+1)
		var needed uint32
		err := windows.NtQuerySystemInformation(windows.SystemModuleInformation, unsafe.Pointer(&buffer[0]), uint32(len(buffer)*8), &needed)
		if err == windows.STATUS_INFO_LENGTH_MISMATCH && needed > size {
			size = needed + 4096
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("list the drivers: %w", err)
		}
		list := (*windows.RTL_PROCESS_MODULES)(unsafe.Pointer(&buffer[0]))
		names := make([]string, 0, list.NumberOfModules)
		for _, module := range unsafe.Slice(&list.Modules[0], list.NumberOfModules) {
			path := module.FullPathName[:]
			if int(module.OffsetToFileName) < len(path) {
				names = append(names, windows.ByteSliceToString(path[module.OffsetToFileName:]))
			}
		}
		return names, nil
	}
}
