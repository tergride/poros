package boxmain

import (
	"context"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/experimental/deprecated"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing/service"
)

var disableColor bool

// Must stay per-box: boxes sharing one service.Registry each overwrite the previous box's MustRegister[OutboundManager].
func newBoxContext() context.Context {
	ctx := service.ContextWith(context.Background(), deprecated.NewStderrManager(log.StdLogger()))
	ctx = box.Context(ctx, include.InboundRegistry(), include.OutboundRegistry(), include.EndpointRegistry(), include.DNSTransportRegistry(), include.ServiceRegistry(), include.CertificateProviderRegistry())
	return ctx
}
