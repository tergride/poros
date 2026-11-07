package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

// remoteDNS reads the remote DNS server of a preset into the dns-remote server, dialed through the server. A name
// in its address is resolved with the system's DNS: the remote one cannot resolve its own address.
func remoteDNS(text string) (option.DNSServerOptions, error) {
	invalid := fmt.Errorf("%q is not a DNS server: write https://host/path, tls://host or an address", text)
	text = strings.TrimSpace(text)
	kind, rest, hasScheme := strings.Cut(text, "://")
	if !hasScheme {
		kind, rest = "udp", text
	}
	parsed, err := url.Parse("dns://" + rest)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return option.DNSServerOptions{}, invalid
	}
	host := ace(parsed.Hostname())
	var port uint16
	if text := parsed.Port(); text != "" {
		value, err := strconv.ParseUint(text, 10, 16)
		if err != nil || value == 0 {
			return option.DNSServerOptions{}, invalid
		}
		port = uint16(value)
	}
	dialer := option.DialerOptions{Detour: TagProxy}
	if net.ParseIP(host) == nil {
		dialer.DomainResolver = &option.DomainResolveOptions{Server: dnsDirect}
	}
	remote := option.RemoteDNSServerOptions{
		RawLocalDNSServerOptions: option.RawLocalDNSServerOptions{DialerOptions: dialer},
		DNSServerAddressOptions:  option.DNSServerAddressOptions{Server: host, ServerPort: port},
	}
	switch strings.ToLower(kind) {
	case "https":
		path := parsed.EscapedPath()
		if path == "" || path == "/" {
			path = remoteDNSPath
		}
		return option.DNSServerOptions{Type: C.DNSTypeHTTPS, Tag: dnsRemote, Options: &option.RemoteHTTPSDNSServerOptions{
			RemoteTLSDNSServerOptions: option.RemoteTLSDNSServerOptions{RemoteDNSServerOptions: remote},
			Path:                      path,
		}}, nil
	case "tls":
		if parsed.Path != "" {
			return option.DNSServerOptions{}, invalid
		}
		return option.DNSServerOptions{Type: C.DNSTypeTLS, Tag: dnsRemote, Options: &option.RemoteTLSDNSServerOptions{RemoteDNSServerOptions: remote}}, nil
	case "udp":
		if parsed.Path != "" || net.ParseIP(host) == nil {
			return option.DNSServerOptions{}, invalid
		}
		return option.DNSServerOptions{Type: C.DNSTypeUDP, Tag: dnsRemote, Options: &remote}, nil
	}
	return option.DNSServerOptions{}, invalid
}
