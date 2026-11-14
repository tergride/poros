package config

import (
	"fmt"
	"math/rand/v2"
	"net/netip"
	"strings"
	"testing"
	"time"

	"PorosCore/internal/boxmain"
	"PorosCore/internal/xray"
)

// settingAxis is one setting of a connection with the values it takes.
type settingAxis struct {
	name   string
	values []string
	apply  func(in *Input, value string)
}

// pairwiseCases picks value indexes so that every pair of values of two axes meets in at least one case, greedily
// and with a fixed seed: the same cases come every run.
func pairwiseCases(axes []settingAxis, seed uint64) [][]int {
	type pair struct{ a, va, b, vb int }
	var order []pair
	uncovered := map[pair]bool{}
	for a := range axes {
		for b := a + 1; b < len(axes); b++ {
			for va := range axes[a].values {
				for vb := range axes[b].values {
					p := pair{a, va, b, vb}
					order = append(order, p)
					uncovered[p] = true
				}
			}
		}
	}
	covers := func(c []int, p pair) bool { return c[p.a] == p.va && c[p.b] == p.vb }
	rng := rand.New(rand.NewPCG(seed, 1))
	var cases [][]int
	for len(uncovered) > 0 {
		var first pair
		for _, p := range order {
			if uncovered[p] {
				first = p
				break
			}
		}
		var best []int
		bestGain := -1
		for range 60 {
			c := make([]int, len(axes))
			for i := range c {
				c[i] = rng.IntN(len(axes[i].values))
			}
			c[first.a], c[first.b] = first.va, first.vb
			gain := 0
			for p := range uncovered {
				if covers(c, p) {
					gain++
				}
			}
			if gain > bestGain {
				best, bestGain = c, gain
			}
		}
		for p := range uncovered {
			if covers(best, p) {
				delete(uncovered, p)
			}
		}
		cases = append(cases, best)
	}
	return cases
}

// Every pair of values of the settings of a connection meets in at least one build, and each build passes the
// checks of sing-box and of Xray: a setting that breaks only next to another shows here.
func TestBuildSettingsPairwise(t *testing.T) {
	links := sampleLinks(t)
	on := func(value string) bool { return value == "on" }
	onOff := []string{"off", "on"}
	axes := []settingAxis{
		{"server", []string{"trojan tcp", "vless reality vision", "vless ws tls", "vmess grpc", "ss 2022 uot", "hysteria2", "wireguard", "openvpn"},
			func(in *Input, v string) { in.Server = mustParse(t, links[v]) }},
		{"REALITY on Xray", onOff, func(in *Input, v string) { in.RealityOnXray = on(v) }},
		{"refuse", onOff, func(in *Input, v string) { in.Refuse = on(v) }},
		{"TUN", onOff, func(in *Input, v string) { in.Tun = on(v) }},
		{"loose route", onOff, func(in *Input, v string) { in.TunOptions.LooseRoute = on(v) }},
		{"TUN routing", onOff, func(in *Input, v string) { in.TunOptions.Routing = on(v) }},
		{"TUN MTU", []string{"default", "1280"}, func(in *Input, v string) {
			if v == "1280" {
				in.TunOptions.MTU = 1280
			}
		}},
		{"allow LAN", onOff, func(in *Input, v string) { in.AllowLAN = on(v) }},
		{"password", onOff, func(in *Input, v string) {
			if on(v) {
				in.Auth, in.SystemPort = &ProxyAuth{Username: "user", Password: "pass"}, 2091
			}
		}},
		{"no IPv6", onOff, func(in *Input, v string) { in.DisableIPv6 = on(v) }},
		{"DNS routing", []string{RoutingAuto, RoutingProxy, RoutingDirect}, func(in *Input, v string) { in.DNS.Routing = v }},
		{"FakeIP", onOff, func(in *Input, v string) { in.DNS.FakeIP = on(v) }},
		{"hosts", onOff, func(in *Input, v string) { in.DNS.Hosts = on(v) }},
		{"answers", onOff, func(in *Input, v string) {
			if on(v) {
				in.DNS.Answers = []Answer{{Name: "router.lan", Addresses: []netip.Addr{netip.MustParseAddr("192.168.1.1")}}}
			}
		}},
		{"address first", []string{"", AddressIPv4, AddressIPv6}, func(in *Input, v string) { in.DNS.AddressFirst = v }},
		{"cache", []string{"default", "none", "tuned"}, func(in *Input, v string) {
			switch v {
			case "none":
				in.DNS.NoCache = true
			case "tuned":
				in.DNS.CacheSize, in.DNS.Optimistic, in.DNS.Timeout = 1024, true, 5*time.Second
				in.DNS.ClientSubnet = netip.MustParsePrefix("203.0.113.0/24")
			}
		}},
		{"default outbound", []string{OutboundProxy, OutboundDirect, OutboundBlock}, func(in *Input, v string) { in.Preset.DefaultOutbound = v }},
		{"remote DNS", []string{"", "tls://1.1.1.1", "9.9.9.9"}, func(in *Input, v string) { in.Preset.DNS = v }},
		{"direct DNS", []string{"", "1.1.1.1"}, func(in *Input, v string) { in.Preset.DirectDNS = v }},
		{"tuning", []string{"none", "hardened", "multiplex", "fragment", "insecure"}, func(in *Input, v string) {
			switch v {
			case "hardened":
				in.Tuning.Hardened = true
			case "multiplex":
				in.Tuning.Multiplex, in.Tuning.MuxProtocol, in.Tuning.MuxStreams = true, "h2mux", 4
			case "fragment":
				in.Tuning.Fragment, in.Tuning.Fingerprint = true, "chrome"
			case "insecure":
				in.Tuning.Insecure, in.Tuning.XraySystemCA = true, true
			}
		}},
		{"NTP", []string{"system", "direct", "proxy"}, func(in *Input, v string) {
			if v != "system" {
				in.NTP = &NTP{Server: "time.windows.com", Port: 123, Interval: 30 * time.Minute, Proxy: v == "proxy"}
			}
		}},
		{"statistics", onOff, func(in *Input, v string) { in.FindProcess = on(v) }},
		{"debug log", onOff, func(in *Input, v string) {
			if on(v) {
				in.LogLevel = "debug"
			}
		}},
	}
	cases := pairwiseCases(axes, 20261114)
	t.Logf("%d builds cover every pair of %d settings", len(cases), len(axes))
	for i, c := range cases {
		in := Input{Preset: allPreset, MixedPort: 2090, Bridge: testBridge, Interface: "Ethernet", Resolved: map[string]netip.Addr{}}
		var described []string
		for a, axis := range axes {
			axis.apply(&in, axis.values[c[a]])
			described = append(described, fmt.Sprintf("%s=%s", axis.name, axis.values[c[a]]))
		}
		what := strings.Join(described, ", ")
		configs, err := Build(in)
		if err != nil {
			t.Errorf("build %d (%s): %v", i, what, err)
			continue
		}
		data, err := configs.SingBoxJSON()
		if err != nil {
			t.Errorf("build %d (%s): %v", i, what, err)
			continue
		}
		if err = boxmain.Check(data); err != nil {
			t.Errorf("build %d (%s): sing-box refuses it: %v", i, what, err)
		}
		if configs.Xray != nil {
			if err = xray.CheckXrayConfig(string(configs.Xray)); err != nil {
				t.Errorf("build %d (%s): Xray refuses it: %v", i, what, err)
			}
		}
	}
}
