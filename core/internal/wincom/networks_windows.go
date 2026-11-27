package wincom

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	clsidNetworkListManager = windows.GUID{Data1: 0xdcb00c01, Data2: 0x570f, Data3: 0x4a9b, Data4: [8]byte{0x8d, 0x69, 0x19, 0x9f, 0xdb, 0xa5, 0x72, 0x3b}}
	iidNetworkListManager   = windows.GUID{Data1: 0xdcb00000, Data2: 0x570f, Data3: 0x4a9b, Data4: [8]byte{0x8d, 0x69, 0x19, 0x9f, 0xdb, 0xa5, 0x72, 0x3b}}
)

// The places of the methods in the tables of INetworkListManager, IEnumNetworks and INetwork, after IDispatch.
const (
	managerGetNetworks = 7
	enumNext           = 8
	networkGetName     = 7
	networkGetID       = 11
	networkIsConnected = 16
	enumNetworkAll     = 3
)

// Network is a network Windows knows, connected now or remembered from before.
type Network struct {
	// ID is that of its profile, the key of the profile under NetworkList\Profiles.
	ID        windows.GUID
	Name      string
	Connected bool
}

// Networks lists the networks of Windows, those not connected now too. It needs no administrator rights.
func Networks() ([]Network, error) {
	manager, err := create(&clsidNetworkListManager, &iidNetworkListManager, clsctxInprocServer|clsctxLocalServer)
	if err != nil {
		return nil, err
	}
	defer manager.Release()
	var list *Object
	if err := manager.Call(managerGetNetworks, enumNetworkAll, uintptr(unsafe.Pointer(&list))); err != nil {
		return nil, err
	}
	defer list.Release()
	var networks []Network
	for {
		var network *Object
		var fetched uint32
		if err := list.Call(enumNext, 1, uintptr(unsafe.Pointer(&network)), uintptr(unsafe.Pointer(&fetched))); err != nil {
			return nil, err
		}
		if fetched == 0 || network == nil {
			return networks, nil
		}
		found, err := describe(network)
		network.Release()
		if err != nil {
			return nil, err
		}
		networks = append(networks, found)
	}
}

func describe(network *Object) (Network, error) {
	var found Network
	var name *uint16
	if err := network.Call(networkGetName, uintptr(unsafe.Pointer(&name))); err != nil {
		return found, err
	}
	found.Name = takeString(name)
	if err := network.Call(networkGetID, uintptr(unsafe.Pointer(&found.ID))); err != nil {
		return found, err
	}
	var connected int16
	if err := network.Call(networkIsConnected, uintptr(unsafe.Pointer(&connected))); err != nil {
		return found, err
	}
	found.Connected = connected != 0
	return found, nil
}
