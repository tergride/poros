package config

import (
	"net/netip"

	"PorosCore/profile"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badjson"
	"github.com/sagernet/sing/common/json/badoption"
)

// TestServer is a server to measure, under its own tag.
type TestServer struct {
	Tag    string
	Server profile.Profile
	// Links are the servers of a chain, in its order.
	Links []profile.Profile
	// Bridge is needed by a server that runs on Xray, or a chain whose first server does.
	Bridge Bridge
}

// TestInput is a set of servers to measure at once.
type TestInput struct {
	Servers       []TestServer
	RealityOnXray bool
	// Interface serves the servers on Xray, as for a connection. Resolved holds the addresses of the servers' names,
	// for Xray and sing-box alike.
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
	var endpoints []option.Endpoint
	var entries []xrayEntry
	for _, server := range in.Servers {
		if err := validateServer(server.Server, server.Links); err != nil {
			configs.Failed[server.Tag] = err
			continue
		}
		var bridge *option.Outbound
		var entry *xrayEntry
		if xrayServer := Entry(server.Server, server.Links); RunsOnXray(xrayServer, in.RealityOnXray) {
			tag := entryTag(server.Server, server.Tag)
			outbound, err := xrayVLESS(xrayServer, tag, in.Interface, in.Resolved)
			if err != nil {
				configs.Failed[server.Tag] = err
				continue
			}
			entry = &xrayEntry{tag: tag, bridge: server.Bridge, outbound: outbound}
			b := bridgeOutbound(tag, server.Bridge)
			bridge = &b
		}
		built, err := serverParts(server.Server, server.Links, server.Tag, bridge, false)
		if err != nil {
			configs.Failed[server.Tag] = err
			continue
		}
		if entry != nil {
			entries = append(entries, *entry)
		}
		outbounds = append(outbounds, built.outbounds...)
		endpoints = append(endpoints, built.endpoints...)
	}
	if len(entries) > 0 {
		xray, err := xrayConfig(entries)
		if err != nil {
			return TestConfigs{}, err
		}
		configs.Xray = xray
	}
	servers := []option.DNSServerOptions{{Type: C.DNSTypeLocal, Tag: TagDNSDirect, Options: &option.LocalDNSServerOptions{}}}
	resolver := TagDNSDirect
	if len(in.Resolved) > 0 {
		servers = append(servers, testHosts(in.Resolved))
		resolver = dnsTestHosts
	}
	configs.SingBox = option.Options{
		Log:       &option.LogOptions{Level: "error"},
		DNS:       &option.DNSOptions{RawDNSOptions: option.RawDNSOptions{Servers: servers}},
		Outbounds: append(outbounds, option.Outbound{Type: C.TypeDirect, Tag: TagDirect, Options: &option.DirectOutboundOptions{}}),
		Endpoints: endpoints,
		Route: &option.RouteOptions{
			Final:                 TagDirect,
			AutoDetectInterface:   true,
			DefaultDomainResolver: &option.DomainResolveOptions{Server: resolver},
		},
	}
	return configs, nil
}

// dnsTestHosts answers the servers' names with the addresses the engine looked up: the system's DNS may be held by
// the TUN of another client, which a test dials past.
const dnsTestHosts = "dns-test-hosts"

func testHosts(resolved map[string]netip.Addr) option.DNSServerOptions {
	predefined := &badjson.TypedMap[string, badoption.Listable[netip.Addr]]{}
	for host, addr := range resolved {
		predefined.Put(host, badoption.Listable[netip.Addr]{addr})
	}
	return option.DNSServerOptions{Type: C.DNSTypeHosts, Tag: dnsTestHosts, Options: &option.HostsDNSServerOptions{Predefined: predefined}}
}

// SingBoxJSON writes the sing-box config the way the core reads it.
func (c TestConfigs) SingBoxJSON() ([]byte, error) {
	return Configs{SingBox: c.SingBox}.SingBoxJSON()
}
