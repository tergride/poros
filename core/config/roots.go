package config

import (
	"net/netip"
	"slices"
	"strings"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// publicDNS are Google, Cloudflare and Quad9, the DNS of the default and of the menus, by address and by name.
var publicDNS = []string{
	"8.8.8.8", "8.8.4.4", "2001:4860:4860::8888", "2001:4860:4860::8844", "dns.google",
	"1.1.1.1", "1.0.0.1", "2606:4700:4700::1111", "2606:4700:4700::1001", "one.one.one.one", "cloudflare-dns.com",
	"9.9.9.9", "149.112.112.112", "2620:fe::fe", "2620:fe::9", "dns.quad9.net",
}

// dnsTLS is how a DNS over TLS verifies its certificate. A public one verifies by dnsRoots, without the system,
// which on Windows makes the first names wait some 20 seconds. Any other DNS is left to the system.
func dnsTLS(host string) option.OutboundTLSOptionsContainer {
	name := strings.TrimSuffix(strings.ToLower(host), ".")
	if addr, err := netip.ParseAddr(name); err == nil {
		name = addr.Unmap().String()
	}
	if !slices.Contains(publicDNS, name) {
		return option.OutboundTLSOptionsContainer{}
	}
	roots := dnsRoots()
	if roots == "" {
		return option.OutboundTLSOptionsContainer{}
	}
	return option.OutboundTLSOptionsContainer{TLS: &option.OutboundTLSOptions{
		Enabled:     true,
		Certificate: badoption.Listable[string]{roots},
	}}
}
