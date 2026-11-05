package sysproxy

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wininet             = windows.NewLazySystemDLL("wininet.dll")
	internetSetOption   = wininet.NewProc("InternetSetOptionW")
	internetQueryOption = wininet.NewProc("InternetQueryOptionW")
	rasapi32            = windows.NewLazySystemDLL("rasapi32.dll")
	rasEnumEntries      = rasapi32.NewProc("RasEnumEntriesW")
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	globalFree          = kernel32.NewProc("GlobalFree")
)

const (
	optionRefresh              = 37
	optionSettingsChanged      = 39
	optionPerConnectionOption  = 75
	optionProxySettingsChanged = 95

	perConnFlags       = 1
	perConnProxyServer = 2
	perConnFlagsUI     = 10

	errorBufferTooSmall = 603
	rasMaxEntryName     = 256
	maxPath             = 260
)

// perConnOption is INTERNET_PER_CONN_OPTIONW: the union is a DWORD, a string or a FILETIME, eight bytes.
type perConnOption struct {
	option uint32
	value  uint64
}

func (o *perConnOption) setDWORD(value uint32) { *(*uint32)(unsafe.Pointer(&o.value)) = value }
func (o *perConnOption) dword() uint32         { return *(*uint32)(unsafe.Pointer(&o.value)) }
func (o *perConnOption) setString(value *uint16) {
	*(**uint16)(unsafe.Pointer(&o.value)) = value
}
func (o *perConnOption) string() *uint16 { return *(**uint16)(unsafe.Pointer(&o.value)) }

// perConnOptionList is INTERNET_PER_CONN_OPTION_LISTW.
type perConnOptionList struct {
	size        uint32
	connection  *uint16
	optionCount uint32
	optionError uint32
	options     *perConnOption
}

// rasEntryName is RASENTRYNAMEW.
type rasEntryName struct {
	size          uint32
	entryName     [rasMaxEntryName + 1]uint16
	flags         uint32
	phonebookPath [maxPath + 1]uint16
}

type winINet struct{}

func platformBackend() backend { return winINet{} }

// connections lists the LAN settings and every dial-up, PPPoE and VPN entry: WinINet keeps a proxy for each and
// uses the active one's.
func (winINet) connections() ([]string, error) {
	names := []string{""}
	var size, count uint32
	ret, _, _ := rasEnumEntries.Call(0, 0, 0, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)))
	if ret == 0 {
		return names, nil
	}
	if ret != errorBufferTooSmall {
		return names, fmt.Errorf("RasEnumEntries failed (%d), only the LAN settings changed", ret)
	}
	entries := make([]rasEntryName, size/uint32(unsafe.Sizeof(rasEntryName{}))+1)
	entries[0].size = uint32(unsafe.Sizeof(rasEntryName{}))
	size = uint32(len(entries)) * uint32(unsafe.Sizeof(rasEntryName{}))
	ret, _, _ = rasEnumEntries.Call(0, 0, uintptr(unsafe.Pointer(&entries[0])), uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)))
	if ret != 0 {
		return names, fmt.Errorf("RasEnumEntries failed (%d), only the LAN settings changed", ret)
	}
	for i := uint32(0); i < count && int(i) < len(entries); i++ {
		names = append(names, windows.UTF16ToString(entries[i].entryName[:]))
	}
	return names, nil
}

func optionList(connection string, options []perConnOption) (perConnOptionList, error) {
	list := perConnOptionList{optionCount: uint32(len(options)), options: &options[0]}
	list.size = uint32(unsafe.Sizeof(list))
	if connection != "" {
		name, err := windows.UTF16PtrFromString(connection)
		if err != nil {
			return list, err
		}
		list.connection = name
	}
	return list, nil
}

func query(connection string, option *perConnOption) bool {
	options := []perConnOption{*option}
	list, err := optionList(connection, options)
	if err != nil {
		return false
	}
	size := list.size
	ret, _, _ := internetQueryOption.Call(0, optionPerConnectionOption, uintptr(unsafe.Pointer(&list)), uintptr(unsafe.Pointer(&size)))
	runtime.KeepAlive(list.connection)
	*option = options[0]
	return ret != 0
}

// flags reads FLAGS_UI and falls back to FLAGS, as Microsoft advises; only FLAGS is ever written.
func (winINet) flags(connection string) uint32 {
	for _, which := range []uint32{perConnFlagsUI, perConnFlags} {
		option := perConnOption{option: which}
		if query(connection, &option) {
			return option.dword()
		}
	}
	return flagDirect
}

func (winINet) server(connection string) string {
	option := perConnOption{option: perConnProxyServer}
	if !query(connection, &option) || option.string() == nil {
		return ""
	}
	value := option.string()
	server := windows.UTF16PtrToString(value)
	globalFree.Call(uintptr(unsafe.Pointer(value)))
	return server
}

func (winINet) set(connection string, flags uint32, server string) error {
	options := make([]perConnOption, 2)
	options[0].option = perConnFlags
	options[0].setDWORD(flags)
	options[1].option = perConnProxyServer
	var serverText *uint16
	if server != "" {
		var err error
		if serverText, err = windows.UTF16PtrFromString(server); err != nil {
			return err
		}
		options[1].setString(serverText)
	}
	list, err := optionList(connection, options)
	if err != nil {
		return err
	}
	ret, _, callErr := internetSetOption.Call(0, optionPerConnectionOption, uintptr(unsafe.Pointer(&list)), uintptr(list.size))
	runtime.KeepAlive(serverText)
	runtime.KeepAlive(list.connection)
	if ret == 0 {
		name := connection
		if name == "" {
			name = "LAN"
		}
		return fmt.Errorf("%s: InternetSetOption failed: %w", name, callErr)
	}
	return nil
}

func (winINet) announce() {
	for _, option := range []uintptr{optionSettingsChanged, optionProxySettingsChanged, optionRefresh} {
		internetSetOption.Call(0, option, 0, 0)
	}
}
