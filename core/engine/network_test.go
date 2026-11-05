package engine

import (
	"testing"

	"PorosCore/internal/netcheck"
)

func TestLeakIsTheHomeProvider(t *testing.T) {
	home := netcheck.Info{IP: "198.51.100.20", ISP: "Home Telecom", ASN: 64500}
	vpnDNS := netcheck.Info{IP: "192.0.2.53", ISP: "Example DNS", ASN: 64510}
	homeDNS := netcheck.Info{IP: "198.51.100.53", ISP: "Home Telecom", ASN: 64500}

	servers, leak := dnsServers([]netcheck.Info{vpnDNS}, home)
	if leak != LeakNone || servers[0].Home {
		t.Errorf("only the VPN's resolvers: %v, %+v", leak, servers)
	}
	servers, leak = dnsServers([]netcheck.Info{vpnDNS, homeDNS}, home)
	if leak != LeakFound || !servers[1].Home || servers[0].Home {
		t.Errorf("a resolver of the home provider is a leak: %v, %+v", leak, servers)
	}
	// Without an AS number the provider's name decides.
	_, leak = dnsServers([]netcheck.Info{{IP: "198.51.100.53", ISP: "home telecom"}}, netcheck.Info{ISP: "Home Telecom"})
	if leak != LeakFound {
		t.Errorf("the same provider by name: %v", leak)
	}
	// Without the home address there is no telling.
	if _, leak = dnsServers([]netcheck.Info{vpnDNS}, netcheck.Info{}); leak != LeakUnknown {
		t.Errorf("unknown home: %v", leak)
	}
}

func TestNetworkCheckWaitsForTheUser(t *testing.T) {
	e, events := openTest(t, t.TempDir())
	settings := DefaultSettings()
	settings.CheckIP = false
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CheckNetwork(); reason(err) != ReasonCheckOff {
		t.Errorf("a check the user turned off must be refused, got %v", err)
	}
	// Without StartNetworkChecks the engine sends nothing to outside services on its own, as in every other test.
	settings.CheckIP = true
	before := len(*events)
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	for _, ev := range (*events)[before:] {
		if ev.method == EventNetworkChanged {
			t.Fatalf("a check ran before StartNetworkChecks: %+v", ev.params)
		}
	}
	if state := e.NetworkState(); state.Checking || state.CheckedAt != 0 || state.DNS == nil {
		t.Errorf("state before any check: %+v", state)
	}
}
