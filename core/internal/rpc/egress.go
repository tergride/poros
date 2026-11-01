package rpc

import (
	"context"
	"errors"

	"PorosCore/gen"
	"PorosCore/internal/netmon"

	"github.com/sagernet/sing/common/control"
)

func defaultInterfaceName() string {
	ifc := netmon.DefaultInterface()
	if ifc == nil {
		return ""
	}
	return ifc.Name
}

func init() {
	monitor := netmon.Monitor()
	if monitor == nil {
		return
	}
	monitor.RegisterCallback(func(ifc *control.Interface, _ int) {
		name := ""
		if ifc != nil {
			name = ifc.Name
		}
		for _, inst := range liveXrayInstances() {
			inst.SetEgress(name, 0)
		}
	})
}

func (s *server) GetDefaultInterface(ctx context.Context, in *gen.EmptyReq) (*gen.GetDefaultInterfaceResponse, error) {
	ifc := netmon.DefaultInterface()
	if ifc == nil {
		return nil, errors.New("no default interface")
	}
	return &gen.GetDefaultInterfaceResponse{
		Name:  To(ifc.Name),
		Index: To(int32(ifc.Index)),
	}, nil
}
