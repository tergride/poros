package config

import (
	"errors"
	"net/netip"
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
