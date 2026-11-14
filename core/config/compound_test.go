package config

import (
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/option"

	"PorosCore/internal/boxmain"
	"PorosCore/internal/xray"
	"PorosCore/profile"
)

func detourOf(t *testing.T, options any) string {
	t.Helper()
	switch o := options.(type) {
	case *option.TrojanOutboundOptions:
		return o.Detour
	case *option.WireGuardEndpointOptions:
		return o.Detour
	case *option.SOCKSOutboundOptions:
		return o.Detour
	}
	t.Fatalf("no detour in %T", options)
	return ""
}

// Next to the TUN of another client the engine finds the servers' addresses past its fake DNS. What dials a server
// itself takes them, a later link of a chain goes through the one before, and what has a name without an address
// keeps the system's DNS, as does every server without Resolved.
func TestBuildPinsServers(t *testing.T) {
	links := sampleLinks(t)
	trojan, wireGuard, openVPN := mustParse(t, links["trojan tcp"]), mustParse(t, links["wireguard"]), mustParse(t, links["openvpn"])
	if hosts := ServerHosts(wireGuard, []profile.Profile{openVPN}, false); !slices.Equal(hosts, []string{"wg.example.com", "o1.example.com", "o2.example.com"}) {
		t.Fatalf("ServerHosts = %v", hosts)
	}
	if hosts := ServerHosts(mustParse(t, links["vless reality vision"]), nil, true); len(hosts) != 0 {
		t.Fatalf("Xray resolves its own server: %v", hosts)
	}
	pair := profile.Profile{Type: profile.TypeChain, Name: "Pair", Chain: &profile.Chain{Servers: []string{"w", "t"}}}
	in := Input{
		Server: pair, Links: []profile.Profile{wireGuard, trojan}, Preset: Preset{Name: "Office", DefaultOutbound: OutboundProxy,
			Endpoints: []string{"ovpn"}, Rules: allPreset.Rules}, Tun: true, MixedPort: 2090, Endpoints: []profile.Profile{openVPN},
		Resolved: map[string]netip.Addr{
			"wg.example.com": netip.MustParseAddr("192.0.2.1"),
			"t.example.com":  netip.MustParseAddr("192.0.2.2"),
			"o1.example.com": netip.MustParseAddr("192.0.2.3"),
		},
	}
	resolverOf := func(raw any) string {
		if resolver := raw.(option.DialerOptionsWrapper).TakeDialerOptions().DomainResolver; resolver != nil {
			return resolver.Server
		}
		return ""
	}
	build := func() (Configs, string) {
		t.Helper()
		configs, err := Build(in)
		if err != nil {
			t.Fatal(err)
		}
		data, err := configs.SingBoxJSON()
		if err != nil {
			t.Fatal(err)
		}
		if err = boxmain.Check(data); err != nil {
			t.Fatalf("sing-box refuses the config: %v\n%s", err, data)
		}
		return configs, string(data)
	}
	configs, data := build()
	endpoints, out := configs.SingBox.Endpoints, configs.SingBox.Outbounds
	if len(endpoints) != 2 || endpoints[0].Tag != "proxy-1" || endpoints[1].Tag != "endpoint-1" || out[0].Tag != TagProxy {
		t.Fatalf("outbounds %+v, endpoints %+v", out, endpoints)
	}
	if got := resolverOf(endpoints[0].Options); got != dnsServerHosts {
		t.Errorf("the first link dials its server with the engine's address: %q", got)
	}
	if got := resolverOf(out[0].Options); got != "" {
		t.Errorf("the second link goes through the first: %q", got)
	}
	if got := resolverOf(endpoints[1].Options); got != "" {
		t.Errorf("OpenVPN has a further server without an address: %q", got)
	}
	has := func(data, host, addr string) bool {
		return strings.Contains(data, `"`+host+`":"`+addr+`"`) || strings.Contains(data, `"`+host+`":["`+addr+`"]`)
	}
	if !has(data, "wg.example.com", "192.0.2.1") || strings.Contains(data, "192.0.2.2") || strings.Contains(data, "192.0.2.3") {
		t.Errorf("the hosts server holds only the names that dial with it:\n%s", data)
	}

	in.Resolved["o2.example.com"] = netip.MustParseAddr("192.0.2.4")
	if configs, data = build(); resolverOf(configs.SingBox.Endpoints[1].Options) != dnsServerHosts || !has(data, "o2.example.com", "192.0.2.4") {
		t.Errorf("OpenVPN with all its servers resolved:\n%s", data)
	}
	in.Resolved = nil
	if configs, data = build(); resolverOf(configs.SingBox.Endpoints[0].Options) != "" || strings.Contains(data, dnsServerHosts) {
		t.Errorf("without Resolved the system's DNS resolves every server:\n%s", data)
	}
}

// A chain starting on Xray: the bridge to it is the first link, the others go one through another to the last.
func TestBuildChain(t *testing.T) {
	links := sampleLinks(t)
	reality, trojan, wireGuard := mustParse(t, links["vless reality vision"]), mustParse(t, links["trojan tcp"]), mustParse(t, links["wireguard"])
	chain := profile.Profile{Type: profile.TypeChain, Name: "Chain", Chain: &profile.Chain{Servers: []string{"r", "t", "w"}}}
	in := Input{
		Server: chain, Links: []profile.Profile{reality, trojan, wireGuard}, Preset: allPreset, MixedPort: 2090,
		RealityOnXray: true, Bridge: testBridge, Interface: "Ethernet", Resolved: map[string]netip.Addr{},
	}
	configs, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	out := configs.SingBox.Outbounds
	endpoints := configs.SingBox.Endpoints
	if len(out) != 3 || out[0].Tag != "proxy-1" || out[0].Type != "socks" || out[1].Tag != "proxy-2" || detourOf(t, out[1].Options) != "proxy-1" ||
		len(endpoints) != 1 || endpoints[0].Tag != TagProxy || detourOf(t, endpoints[0].Options) != "proxy-2" {
		t.Fatalf("outbounds %+v, endpoints %+v", out, endpoints)
	}
	data, err := configs.SingBoxJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err = boxmain.Check(data); err != nil {
		t.Fatalf("sing-box refuses the chain: %v\n%s", err, data)
	}
	if configs.Xray == nil {
		t.Fatal("the first link runs on Xray")
	}
	if err = xray.CheckXrayConfig(string(configs.Xray)); err != nil {
		t.Fatalf("Xray refuses the first link: %v", err)
	}

	// REALITY later in the chain runs on sing-box, which can dial through another server.
	pair := profile.Profile{Type: profile.TypeChain, Name: "Pair", Chain: &profile.Chain{Servers: []string{"a", "b"}}}
	in.Server = pair
	in.Links = []profile.Profile{trojan, reality}
	if configs, err = Build(in); err != nil || configs.Xray != nil {
		t.Errorf("REALITY as the second link: %v, Xray config %v", err, configs.Xray != nil)
	}
	xhttp := mustParse(t, links["vless xhttp reality"])
	in.Links = []profile.Profile{trojan, xhttp}
	if _, err = Build(in); !errors.Is(err, profile.ErrUnsupported) {
		t.Errorf("a server only Xray runs as the second link: %v", err)
	}
	in.Links = []profile.Profile{trojan}
	if _, err = Build(in); err == nil {
		t.Error("a chain with a server gone is built")
	}
	if _, _, err = Export(pair, []profile.Profile{reality, trojan}, true); !errors.Is(err, profile.ErrUnsupported) {
		t.Errorf("exporting a chain that starts on Xray: %v", err)
	}
	if _, data, err = Export(pair, []profile.Profile{trojan, wireGuard}, true); err != nil {
		t.Errorf("exporting a chain on sing-box: %v", err)
	} else if err = boxmain.Check(data); err != nil {
		t.Errorf("the exported chain is no sing-box config: %v\n%s", err, data)
	}

	tests, err := BuildTest(TestInput{RealityOnXray: true, Servers: []TestServer{
		{Tag: "chain", Server: chain, Links: []profile.Profile{reality, trojan, wireGuard}, Bridge: testBridge},
	}})
	if err != nil || len(tests.Failed) > 0 || tests.Xray == nil {
		t.Fatalf("a chain in the latency test: %v, failed %v", err, tests.Failed)
	}
	if data, err = tests.SingBoxJSON(); err == nil {
		err = boxmain.Check(data)
	}
	if err != nil {
		t.Errorf("sing-box refuses the test config of a chain: %v", err)
	}
}
