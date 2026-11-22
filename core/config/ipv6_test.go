package config

import (
	"net/netip"
	"testing"

	mDNS "github.com/miekg/dns"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	R "github.com/sagernet/sing-box/route/rule"
	"github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"

	"PorosCore/internal/boxmain"
)

type testNetwork struct {
	adapter.NetworkManager
	monitor *testMonitor
}

func (n testNetwork) InterfaceMonitor() tun.DefaultInterfaceMonitor { return n.monitor }

type testMonitor struct {
	tun.DefaultInterfaceMonitor
	current *control.Interface
}

func (m *testMonitor) DefaultInterface() *control.Interface { return m.current }

// The names of direct connections get no IPv6 address while the network has none: the TUN takes a connection
// before its way out fails, and a program that got it does not go on to IPv4. The names of the server keep theirs.
func TestDirectNamesWithoutIPv6InTheNetwork(t *testing.T) {
	const empty = "no addresses"
	route := func(outbound string, rule Rule) Rule {
		rule.Action, rule.Outbound = ActionRoute, outbound
		return rule
	}
	directByDefault := Preset{Name: "Direct", DefaultOutbound: OutboundDirect, Rules: []Rule{
		allPreset.Rules[0],
		route(OutboundDirect, Rule{DomainSuffix: []string{"near.proxy.example"}}),
		route(OutboundProxy, Rule{DomainSuffix: []string{"proxy.example"}}),
		route(OutboundProxy, Rule{ProcessName: []string{"game.exe"}}),
	}}
	proxyByDefault := Preset{Name: "Proxy", DefaultOutbound: OutboundProxy, Rules: []Rule{
		allPreset.Rules[0],
		route(OutboundProxy, Rule{DomainSuffix: []string{"far.direct.example"}}),
		route(OutboundDirect, Rule{DomainSuffix: []string{"direct.example"}}),
		route(OutboundDirect, Rule{ProcessName: []string{"game.exe"}}),
	}}
	allButOne := Preset{Name: "All but one", DefaultOutbound: OutboundProxy, Rules: []Rule{
		route(OutboundDirect, Rule{Invert: true, DomainSuffix: []string{"proxy.example"}}),
	}}

	ipv4Only := []string{"192.168.1.5/24", "fe80::5/64", "fd00::5/64"}
	withIPv6 := []string{"192.168.1.5/24", "fe80::5/64", "2001:db8::5/64"}
	for _, c := range []struct {
		what    string
		preset  Preset
		routing string
		change  func(in *Input)
		network []string
		name    string
		query   uint16
		program string
		want    string
	}{
		{"direct by default", directByDefault, "", nil, ipv4Only, "site.example", mDNS.TypeAAAA, "", empty},
		{"its IPv4", directByDefault, "", nil, ipv4Only, "site.example", mDNS.TypeA, "", TagDNSDirect},
		{"with IPv6 in the network", directByDefault, "", nil, withIPv6, "site.example", mDNS.TypeAAAA, "", TagDNSDirect},
		{"with no network", directByDefault, "", nil, nil, "site.example", mDNS.TypeAAAA, "", empty},
		{"a rule of the server", directByDefault, "", nil, ipv4Only, "proxy.example", mDNS.TypeAAAA, "", TagDNSRemote},
		{"a direct rule above it", directByDefault, "", nil, ipv4Only, "near.proxy.example", mDNS.TypeAAAA, "", empty},
		{"a program of the server", directByDefault, "", nil, ipv4Only, "site.example", mDNS.TypeAAAA, "C:/Game/game.exe", TagDNSRemote},
		{"through the server by default", proxyByDefault, "", nil, ipv4Only, "site.example", mDNS.TypeAAAA, "", TagDNSRemote},
		{"a direct rule", proxyByDefault, "", nil, ipv4Only, "www.direct.example", mDNS.TypeAAAA, "", empty},
		{"a rule of the server above it", proxyByDefault, "", nil, ipv4Only, "far.direct.example", mDNS.TypeAAAA, "", TagDNSRemote},
		{"a direct program", proxyByDefault, "", nil, ipv4Only, "site.example", mDNS.TypeAAAA, "C:/Game/game.exe", empty},
		{"all but a name", allButOne, "", nil, ipv4Only, "site.example", mDNS.TypeAAAA, "", empty},
		{"the name of all but it", allButOne, "", nil, ipv4Only, "proxy.example", mDNS.TypeAAAA, "", TagDNSRemote},
		{"all names through the server, a direct one", directByDefault, RoutingProxy, nil, ipv4Only, "site.example", mDNS.TypeAAAA, "", empty},
		{"all names through the server, one of it", directByDefault, RoutingProxy, nil, ipv4Only, "proxy.example", mDNS.TypeAAAA, "", TagDNSRemote},
		{"all names through the server, IPv4", directByDefault, RoutingProxy, nil, ipv4Only, "site.example", mDNS.TypeA, "", TagDNSRemote},
		{"all names past it, a direct one", proxyByDefault, RoutingDirect, nil, ipv4Only, "direct.example", mDNS.TypeAAAA, "", empty},
		{"all names past it, one of the server", proxyByDefault, RoutingDirect, nil, ipv4Only, "far.direct.example", mDNS.TypeAAAA, "", TagDNSDirect},
		{"all names past it, the rest", proxyByDefault, RoutingDirect, nil, ipv4Only, "site.example", mDNS.TypeAAAA, "", TagDNSDirect},
		{"FakeIP", directByDefault, "", func(in *Input) { in.DNS.FakeIP = true }, ipv4Only, "site.example", mDNS.TypeAAAA, "", tagDNSFake},
		{"without the TUN", directByDefault, "", func(in *Input) { in.Tun = false }, ipv4Only, "site.example", mDNS.TypeAAAA, "", TagDNSDirect},
		{"a refusal", proxyByDefault, "", func(in *Input) { in.Refuse = true }, ipv4Only, "direct.example", mDNS.TypeAAAA, "", empty},
	} {
		t.Run(c.what, func(t *testing.T) {
			in := Input{Server: mustParse(t, sampleLinks(t)["trojan tcp"]), Preset: c.preset, Tun: true, MixedPort: 2090}
			in.DNS.Routing = c.routing
			if c.change != nil {
				c.change(&in)
			}
			configs, err := Build(in)
			if err != nil {
				t.Fatal(err)
			}
			data, err := configs.SingBoxJSON()
			if err != nil {
				t.Fatal(err)
			}
			if err = boxmain.Check(data); err != nil {
				t.Fatalf("sing-box refuses the config: %v", err)
			}

			monitor := &testMonitor{}
			if c.network != nil {
				monitor.current = &control.Interface{Name: "Ethernet"}
				for _, address := range c.network {
					monitor.current.Addresses = append(monitor.current.Addresses, netip.MustParsePrefix(address))
				}
			}
			ctx := service.ContextWith[adapter.NetworkManager](include.Context(t.Context()), testNetwork{monitor: monitor})
			query := adapter.InboundContext{Network: N.NetworkUDP, Domain: c.name, QueryType: c.query}
			if c.program != "" {
				query.ProcessInfo = &adapter.ConnectionOwner{ProcessPaths: []string{c.program}}
			}
			got := configs.SingBox.DNS.Final
			for _, options := range configs.SingBox.DNS.Rules {
				rule, err := R.NewDNSRule(ctx, log.NewNOPFactory().Logger(), options, true, false)
				if err != nil {
					t.Fatal(err)
				}
				asked := query
				if !rule.LegacyPreMatch(&asked) {
					continue
				}
				switch action := rule.Action().(type) {
				case *R.RuleActionDNSRoute:
					got = action.Server
				case *R.RuleActionPredefined:
					answer := action.Response(new(mDNS.Msg).SetQuestion(c.name+".", c.query))
					if answer.Rcode != mDNS.RcodeSuccess || len(answer.Answer) != 0 {
						t.Fatalf("not an answer of no addresses: %v", answer)
					}
					got = empty
				}
				break
			}
			if got != c.want {
				t.Errorf("%s of %s: %s, want %s", mDNS.TypeToString[c.query], c.name, got, c.want)
			}
		})
	}
}

// Without IPv6 the TUN refuses IPv6 before the handshake, so that a program goes on to IPv4 at once, however many
// connections it tries. The router is the real one; the TUN itself is left out.
func TestTunRefusesIPv6BeforeTheHandshake(t *testing.T) {
	verdicts := func(disableIPv6 bool, destinations ...string) []tun.FlowAction {
		t.Helper()
		configs, err := Build(Input{
			Server:      mustParse(t, sampleLinks(t)["trojan tcp"]),
			Preset:      Preset{Name: "Direct", DefaultOutbound: OutboundDirect},
			Tun:         true,
			DisableIPv6: disableIPv6,
			MixedPort:   2090,
		})
		if err != nil {
			t.Fatal(err)
		}
		configs.SingBox.Inbounds = nil
		configs.SingBox.Route.AutoDetectInterface = false
		data, err := configs.SingBoxJSON()
		if err != nil {
			t.Fatal(err)
		}
		box, cancel, err := boxmain.Create(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			cancel()
			box.Close()
		}()
		var got []tun.FlowAction
		for _, destination := range destinations {
			to := netip.MustParseAddrPort(destination)
			from := netip.MustParseAddrPort("172.19.1.2:50000")
			if to.Addr().Is6() {
				from = netip.MustParseAddrPort("[fdfe:dcba:9877::2]:50000")
			}
			const tcp = 6
			metadata := adapter.InboundContext{Inbound: tagTun}
			got = append(got, adapter.JudgeFlow(box.Router(), metadata, tcp, from, to, nil).Action)
		}
		return got
	}

	// More than the 50 refusals in 30 seconds after which sing-box drops in silence by default.
	var many []string
	for range 60 {
		many = append(many, "[2001:db8::7]:443")
	}
	for i, got := range verdicts(true, many...) {
		if got != tun.ActionReject {
			t.Fatalf("connection %d to IPv6: verdict %v, want a refusal before the handshake", i+1, got)
		}
	}
	if got := verdicts(true, "198.51.100.7:443", "[2001:db8::7]:53"); got[0] != tun.ActionAccept || got[1] != tun.ActionAccept {
		t.Errorf("IPv4 and DNS over IPv6 go on: %v", got)
	}
	if got := verdicts(false, "[2001:db8::7]:443"); got[0] != tun.ActionAccept {
		t.Errorf("with IPv6 on it goes on: %v", got)
	}
}
