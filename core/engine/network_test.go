package engine

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
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

// fakeFamilies answers every service with an address per family; a family without one cannot connect.
func fakeFamilies(t *testing.T, v4, v6 string) familyClient {
	t.Helper()
	servers := map[int]*httptest.Server{}
	for family, ip := range map[int]string{4: v4, 6: v6} {
		if ip == "" {
			continue
		}
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"ip":%q,"country_code":"de","city_name":"Nuremberg","isp_name":"Example","as_number":64501}`, ip)
		}))
		t.Cleanup(server.Close)
		servers[family] = server
	}
	return func(family int) (*http.Client, func()) {
		server := servers[family]
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				if server == nil {
					return nil, errors.New("no route")
				}
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			},
		}
		return &http.Client{Transport: transport}, transport.CloseIdleConnections
	}
}

func TestBothAddressesShow(t *testing.T) {
	ctx := t.Context()
	info, home, err := lookupAddresses(ctx, fakeFamilies(t, "203.0.113.4", "2001:db8::4"), true)
	if err != nil || info.IP != "203.0.113.4" || info.IPv6 != "2001:db8::4" || info.CountryCode != "DE" || home.ASN != 64501 {
		t.Errorf("both families: %+v, %v", info, err)
	}
	// Without IPv6 the check does not ask for it.
	if info, _, err = lookupAddresses(ctx, fakeFamilies(t, "203.0.113.4", "2001:db8::4"), false); err != nil || info.IPv6 != "" {
		t.Errorf("IPv6 off: %+v, %v", info, err)
	}
	// A network with one family is no error.
	if info, _, err = lookupAddresses(ctx, fakeFamilies(t, "203.0.113.4", ""), true); err != nil || info.IP != "203.0.113.4" || info.IPv6 != "" {
		t.Errorf("IPv4 only: %+v, %v", info, err)
	}
	if info, _, err = lookupAddresses(ctx, fakeFamilies(t, "", "2001:db8::4"), true); err != nil || info.IP != "" || info.IPv6 != "2001:db8::4" {
		t.Errorf("IPv6 only: %+v, %v", info, err)
	}
	if _, _, err = lookupAddresses(ctx, fakeFamilies(t, "", ""), true); err == nil {
		t.Error("no answer at all is an error")
	}
}
