package config

import (
	"net/netip"

	"PorosCore/profile"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

// TestServer is a server to measure, under its own tag.
type TestServer struct {
	Tag    string
	Server profile.Profile
	// Bridge is needed by a server that runs on Xray.
	Bridge Bridge
}

// TestInput is a set of servers to measure at once.
type TestInput struct {
	Servers       []TestServer
	RealityOnXray bool
	// Interface and Resolved serve the servers on Xray, as for a connection.
	Interface string
	Resolved  map[string]netip.Addr
}

// TestConfigs measure servers: an outbound per server under its tag, in sing-box or behind a bridge to Xray.
// Failed holds the servers that could not be built, by tag.
type TestConfigs struct {
	SingBox option.Options
	Xray    []byte
	Failed  map[string]error
}

// BuildTest builds the configs to measure servers with. Their connections go out of the network adapter, not
// into a running TUN, and nothing listens: the probes dial the outbounds directly.
func BuildTest(in TestInput) (TestConfigs, error) {
	configs := TestConfigs{Failed: map[string]error{}}
	var outbounds []option.Outbound
	var entries []xrayEntry
	for _, server := range in.Servers {
		if err := server.Server.Validate(); err != nil {
			configs.Failed[server.Tag] = err
			continue
		}
		if !RunsOnXray(server.Server, in.RealityOnXray) {
			outbound, err := singBoxOutbound(server.Server, server.Tag, false)
			if err != nil {
				configs.Failed[server.Tag] = err
				continue
			}
			outbounds = append(outbounds, outbound)
			continue
		}
		outbound, err := xrayVLESS(server.Server, server.Tag, in.Interface, in.Resolved)
		if err != nil {
			configs.Failed[server.Tag] = err
			continue
		}
		entries = append(entries, xrayEntry{tag: server.Tag, bridge: server.Bridge, outbound: outbound})
		outbounds = append(outbounds, option.Outbound{Type: C.TypeSOCKS, Tag: server.Tag, Options: &option.SOCKSOutboundOptions{
			ServerOptions: option.ServerOptions{Server: "127.0.0.1", ServerPort: server.Bridge.Port},
			Username:      server.Bridge.Username,
			Password:      server.Bridge.Password,
		}})
	}
	if len(entries) > 0 {
		xray, err := xrayConfig(entries)
		if err != nil {
			return TestConfigs{}, err
		}
		configs.Xray = xray
	}
	configs.SingBox = option.Options{
		Log: &option.LogOptions{Level: "error"},
		DNS: &option.DNSOptions{RawDNSOptions: option.RawDNSOptions{
			Servers: []option.DNSServerOptions{{Type: C.DNSTypeLocal, Tag: dnsDirect, Options: &option.LocalDNSServerOptions{}}},
		}},
		Outbounds: append(outbounds, option.Outbound{Type: C.TypeDirect, Tag: TagDirect, Options: &option.DirectOutboundOptions{}}),
		Route: &option.RouteOptions{
			Final:                 TagDirect,
			AutoDetectInterface:   true,
			DefaultDomainResolver: &option.DomainResolveOptions{Server: dnsDirect},
		},
	}
	return configs, nil
}

// SingBoxJSON writes the sing-box config the way the core reads it.
func (c TestConfigs) SingBoxJSON() ([]byte, error) {
	return Configs{SingBox: c.SingBox}.SingBoxJSON()
}
