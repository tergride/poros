package engine

import (
	"encoding/json"
	"fmt"
	"testing"

	"PorosCore/config"
)

// The DNS page of the settings: what a data folder starts with, what is refused, and the servers presets take by
// default.
func TestDNSSettings(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	settings, err := e.Settings()
	if err != nil || settings.DNSRouting != DNSRoutingAuto || !settings.DNSHosts || !settings.DNSCache || settings.DNSCacheSize != 4096 ||
		settings.DNSTimeout != 10 || settings.FakeIP || settings.DNSRemote != "" || settings.DNSDirect != "" {
		t.Fatalf("the defaults: %+v, %v", settings, err)
	}
	for name, change := range map[string]func(s *Settings){
		"a server that is none":    func(s *Settings) { s.DNSRemote = "dns.example" },
		"a direct one":             func(s *Settings) { s.DNSDirect = "ftp://dns.example" },
		"a way of names":           func(s *Settings) { s.DNSRouting = "both" },
		"a family":                 func(s *Settings) { s.AddressFirst = "ipv5" },
		"an answer":                func(s *Settings) { s.DNSAnswers = "nas.home" },
		"a cache of nothing":       func(s *Settings) { s.DNSCacheSize = 0 },
		"no wait":                  func(s *Settings) { s.DNSTimeout = 0 },
		"a subnet":                 func(s *Settings) { s.DNSClientSubnet = "203.0.113.0/33" },
		"an optimistic eternity":   func(s *Settings) { s.DNSIgnoreTTL, s.DNSOptimistic = true, true },
		"an object of a mistake":   func(s *Settings) { s.DNSObject = `{"servers":[{"type":"local","tag":"local","detour":"proxi"}]}` },
		"an object that is no one": func(s *Settings) { s.DNSObject = `[]` },
	} {
		wrong := settings
		change(&wrong)
		if _, err := e.SetSettings(wrong); reason(err) != ReasonInvalidValue {
			t.Errorf("%s: %v", name, err)
		}
	}
	right := settings
	right.DNSRemote, right.DNSDirect, right.DNSRouting = "tls://1.1.1.1", "https://dns.example/dns-query", DNSRoutingProxy
	right.DNSAnswers, right.DNSClientSubnet, right.AddressFirst = "nas.home 192.168.1.20", "203.0.113.7", AddressFirstIPv4
	right.DNSObject = `{"servers":[{"type":"https","tag":"remote","server":"1.1.1.1","detour":"proxy"}],"final":"remote"}`
	if _, err := e.SetSettings(right); err != nil {
		t.Fatal(err)
	}
	dns := dnsOf(right)
	if dns.Routing != config.RoutingProxy || len(dns.Answers) != 1 || dns.ClientSubnet.String() != "203.0.113.7/32" || dns.Object == "" {
		t.Errorf("the DNS of a connection: %+v", dns)
	}

	// A preset with «By default» takes the servers of the settings; its own stay.
	preset, _, _ := e.preset("all")
	if got := withDefaultDNS(preset, right); got.DNS != "tls://1.1.1.1" || got.DirectDNS != "https://dns.example/dns-query" {
		t.Errorf("the default servers: %+v", got)
	}
	preset.DNS = "https://9.9.9.9/dns-query"
	if got := withDefaultDNS(preset, right); got.DNS != "https://9.9.9.9/dns-query" {
		t.Errorf("the preset's own server: %+v", got)
	}

	check := e.Handlers()[MethodDNSCheck]
	if _, err := check(t.Context(), json.RawMessage(`{"object":"{\"servers\":[{\"type\":\"local\",\"tag\":\"local\"}]}"}`)); err != nil {
		t.Errorf("a right object: %v", err)
	}
	if _, err := check(t.Context(), json.RawMessage(`{"object":"{\"final\":\"remote\"}"}`)); reason(err) != ReasonInvalidValue {
		t.Errorf("a final server the object has not: %v", err)
	}
}

// An own dns object comes into a real connection: the core starts with it and the page opens.
func TestConnectionWithOwnDNSObject(t *testing.T) {
	e, states := openConnectionTest(t)
	page := webPage(t)
	port := freePort(t)
	runServer(t, fmt.Sprintf(`{"log":{"level":"warn"},"inbounds":[{"type":"trojan","listen":"127.0.0.1","listen_port":%d,`+
		`"users":[{"password":"secret"}]}],"outbounds":[{"type":"direct","tag":"direct"}]}`, port))
	useServer(t, e, fmt.Sprintf("trojan://secret@127.0.0.1:%d?security=none#local", port))
	settings, _ := e.Settings()
	settings.DNSObject = `{"servers":[{"type":"https","tag":"remote","server":"1.1.1.1","detour":"proxy"},{"type":"local","tag":"system"}],` +
		`"rules":[{"domain_suffix":["lan"],"server":"system"}],"final":"remote"}`
	settings.FakeIP, settings.DNSRouting = true, DNSRoutingDirect
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	connectAndFetch(t, e, states, page)
}
