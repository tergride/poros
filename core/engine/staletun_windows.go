package engine

import (
	"crypto/md5"
	"encoding/binary"
	"errors"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"PorosCore/config"
)

// netClass is the class of network adapters.
var netClass = windows.GUID{Data1: 0x4d36e972, Data2: 0xe325, Data3: 0x11ce, Data4: [8]byte{0xbf, 0xc1, 0x08, 0x00, 0x2b, 0xe1, 0x03, 0x18}}

// tunGUID is the GUID sing-tun gives the Wintun adapter of a name, so its device is the same on every run.
func tunGUID(name string) windows.GUID {
	sum := md5.Sum([]byte("wintun" + name))
	guid := windows.GUID{
		Data1: binary.LittleEndian.Uint32(sum[0:4]),
		Data2: binary.LittleEndian.Uint16(sum[4:6]),
		Data3: binary.LittleEndian.Uint16(sum[6:8]),
	}
	copy(guid.Data4[:], sum[8:16])
	return guid
}

// removeStaleTun removes the device of Poros's TUN that a run left behind without its process, as a power cut or a
// crash of Windows does: Windows keeps it as a device that is not present. Bringing such a device back stalled the
// next TUN for 15 seconds on a user's Windows 10 and failed it; Wintun then removed the device, and the start after
// that worked. A device that is present belongs to a TUN that runs and stays. It needs administrator rights, as the
// TUN does.
func removeStaleTun() (bool, error) {
	instance := `SWD\WINTUN\` + tunGUID(config.TunName).String()
	// Without DIGCF_PRESENT the list holds the devices that are not present too.
	devices, err := windows.SetupDiGetClassDevsEx(&netClass, "SWD", 0, 0, 0, "")
	if err != nil {
		return false, err
	}
	defer devices.Close()
	for i := 0; ; i++ {
		device, err := devices.EnumDeviceInfo(i)
		if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			return false, nil
		}
		if err != nil {
			continue
		}
		id, err := devices.DeviceInstanceID(device)
		if err != nil || !strings.EqualFold(id, instance) {
			continue
		}
		var status, problem uint32
		if windows.CM_Get_DevNode_Status(&status, &problem, device.DevInst, 0) == nil {
			return false, nil
		}
		params := windows.RemoveDeviceParams{ClassInstallHeader: *windows.MakeClassInstallHeader(windows.DIF_REMOVE), Scope: windows.DI_REMOVEDEVICE_GLOBAL}
		if err = devices.SetClassInstallParams(device, &params.ClassInstallHeader, uint32(unsafe.Sizeof(params))); err != nil {
			return false, err
		}
		if err = devices.CallClassInstaller(windows.DIF_REMOVE, device); err != nil {
			return false, err
		}
		return true, nil
	}
}
