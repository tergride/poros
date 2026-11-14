package main

import (
	"net/netip"
	"strings"
)

// The lab's plan of addresses; the gateway's poros-lab script sets up the same.
var (
	// userPublic is poros-test behind the provider's NAT; its IPv6 addresses are not translated.
	userPublic = netip.MustParseAddr("192.0.2.77")
	userNets   = []netip.Prefix{netip.MustParsePrefix("3fff:0:77::/48"), netip.MustParsePrefix("3fff:0:78::/48")}
	// ispResolver is the provider's DNS (dnsmasq on the gateway) as the internet sees it when it asks upstream.
	ispResolver = []netip.Addr{netip.MustParseAddr("10.200.1.1"), netip.MustParseAddr("fd00:200:1::1")}
	vpnNets     = []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("3fff:0:203::/48")}
)

// names are the lab's DNS: every name the lab answers, by its addresses. A name under a key that starts with "*."
// matches it and its subdomains.
var names = map[string][]string{
	"*.probe.lab": {"198.51.100.10", "3fff:0:100::10"},
	"echo.lab":    {"198.51.100.10", "3fff:0:100::10"},
	"echo2.lab":   {"198.51.100.11", "3fff:0:100::11"},
	"stun.lab":    {"198.51.100.3", "3fff:0:100::3"},
	"sub.lab":     {"198.51.100.20", "3fff:0:100::20"},
	"vpn.lab":     {"203.0.113.10", "3fff:0:203::10"},
	"vpn4.lab":    {"203.0.113.10"},
	"reality.lab": {"198.51.100.30", "3fff:0:100::30"},
	// The services Poros asks itself, by their real names.
	"ipleak.net":           {"198.51.100.30", "3fff:0:100::30"},
	"ipinfo.io":            {"198.51.100.30", "3fff:0:100::30"},
	"ipwho.is":             {"198.51.100.30", "3fff:0:100::30"},
	"api.ip2location.io":   {"198.51.100.30", "3fff:0:100::30"},
	"*.bash.ws":            {"198.51.100.30", "3fff:0:100::30"},
	"bash.ws":              {"198.51.100.30", "3fff:0:100::30"},
	"cp.cloudflare.com":    {"198.51.100.31", "3fff:0:100::31"},
	"speed.cloudflare.com": {"198.51.100.31", "3fff:0:100::31"},
	"time.cloudflare.com":  {"198.51.100.31", "3fff:0:100::31"},
	"github.com":           {"198.51.100.31", "3fff:0:100::31"},
	// What Windows asks to know it is online.
	"www.msftconnecttest.com":  {"198.51.100.32"},
	"ipv6.msftconnecttest.com": {"3fff:0:100::32"},
	"dns.msftncsi.com":         {"131.107.255.255", "fd3e:4f5a:5b81::1"},
}

// lookup gives the addresses of a name, nil for a name the lab does not know.
func lookup(name string) []netip.Addr {
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	texts, ok := names[name]
	for !ok {
		dot := strings.IndexByte(name, '.')
		if dot < 0 {
			return nil
		}
		name = name[dot+1:]
		texts, ok = names["*."+name]
	}
	addrs := make([]netip.Addr, 0, len(texts))
	for _, text := range texts {
		addrs = append(addrs, netip.MustParseAddr(text))
	}
	return addrs
}

// viaOf tells what an address the lab's internet sees stands for.
func viaOf(address string) string {
	addr, err := netip.ParseAddr(hostOf(address))
	if err != nil {
		return "other"
	}
	addr = addr.Unmap()
	switch {
	case addr == userPublic || inAny(addr, userNets):
		return "user"
	case addr == ispResolver[0] || addr == ispResolver[1]:
		return "isp-dns"
	case inAny(addr, vpnNets):
		return "vpn"
	}
	return "other"
}

func inAny(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// place is what an IP lookup service tells about an address of the lab.
type place struct {
	Country string
	City    string
	ISP     string
	ASN     int
}

func placeOf(address string) place {
	switch viaOf(address) {
	case "user", "isp-dns":
		return place{Country: "FI", City: "Lab Home", ISP: "Lab Home ISP", ASN: 64501}
	case "vpn":
		return place{Country: "DE", City: "Lab Exit", ISP: "Lab VPN", ASN: 64502}
	}
	return place{Country: "ZZ", City: "Lab", ISP: "Lab Internet", ASN: 64500}
}
