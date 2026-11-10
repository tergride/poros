package engine

import (
	"net/netip"
	"slices"
	"strings"
	"time"

	"PorosCore/config"
	"PorosCore/internal/boxmain"
	"PorosCore/internal/jsonrpc"
	"PorosCore/profile"
)

// The bounds of the cache and the wait of a DNS query.
const (
	defaultDNSCacheSize = 4096
	maxDNSCacheSize     = 1 << 20
	defaultDNSTimeout   = 10
	maxDNSTimeout       = 60
)

// validateDNS checks the DNS settings: the servers, the ready answers, the bounds of the cache, the subnet and an
// own dns object, which sing-box reads in a connection of its own.
func validateDNS(s Settings) error {
	for name, server := range map[string]string{"dnsRemote": s.DNSRemote, "dnsDirect": s.DNSDirect} {
		if server == "" {
			continue
		}
		if err := config.CheckDNSServer(server, false); err != nil {
			return jsonrpc.Failed(ReasonInvalidValue, "%s: %v", name, err)
		}
	}
	if !slices.Contains([]string{DNSRoutingAuto, DNSRoutingProxy, DNSRoutingDirect}, s.DNSRouting) {
		return jsonrpc.Failed(ReasonInvalidValue, "dnsRouting must be auto, proxy or direct, not %q", s.DNSRouting)
	}
	if !slices.Contains([]string{"", AddressFirstIPv4, AddressFirstIPv6}, s.AddressFirst) {
		return jsonrpc.Failed(ReasonInvalidValue, "addressFirst must be empty, ipv4 or ipv6, not %q", s.AddressFirst)
	}
	if _, err := config.ParseAnswers(s.DNSAnswers); err != nil {
		return jsonrpc.Failed(ReasonInvalidValue, "dnsAnswers: %v", err)
	}
	if s.DNSCacheSize < 1 || s.DNSCacheSize > maxDNSCacheSize {
		return jsonrpc.Failed(ReasonInvalidValue, "dnsCacheSize must be from 1 to %d, not %d", maxDNSCacheSize, s.DNSCacheSize)
	}
	if s.DNSIgnoreTTL && s.DNSOptimistic {
		return jsonrpc.Failed(ReasonInvalidValue, "an optimistic cache needs answers that get outdated: not with dnsIgnoreTtl")
	}
	if s.DNSTimeout < 1 || s.DNSTimeout > maxDNSTimeout {
		return jsonrpc.Failed(ReasonInvalidValue, "dnsTimeout must be from 1 to %d seconds, not %d", maxDNSTimeout, s.DNSTimeout)
	}
	if _, err := clientSubnet(s.DNSClientSubnet); err != nil {
		return jsonrpc.Failed(ReasonInvalidValue, "dnsClientSubnet: %q is no subnet", s.DNSClientSubnet)
	}
	if s.DNSObject != "" {
		return checkDNSObject(s.DNSObject)
	}
	return nil
}

// clientSubnet reads the subnet of ECS: a subnet, or an address as one of its own; "" is none.
func clientSubnet(text string) (netip.Prefix, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return netip.Prefix{}, nil
	}
	if addr, err := netip.ParseAddr(text); err == nil {
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}
	prefix, err := netip.ParsePrefix(text)
	return prefix.Masked(), err
}

// checkDNSObject has sing-box read an own dns object in a connection through a server of its own, as a start
// would, without starting anything.
func checkDNSObject(text string) error {
	if strings.TrimSpace(text) == "" {
		return jsonrpc.Failed(ReasonInvalidValue, "the dns object is empty")
	}
	configs, err := config.Build(config.Input{
		Server:    profile.Profile{Type: profile.TypeDirect, Direct: &profile.Direct{}},
		Preset:    config.Preset{Name: "check", DefaultOutbound: config.OutboundProxy},
		MixedPort: 1,
		DNS:       config.DNS{Object: text},
	})
	var data []byte
	if err == nil {
		data, err = configs.SingBoxJSON()
	}
	if err == nil {
		err = boxmain.Check(data)
	}
	if err != nil {
		return jsonrpc.Failed(ReasonInvalidValue, "%v", err)
	}
	return nil
}

// dnsOf is how a connection resolves names by the settings, which validateDNS has checked.
func dnsOf(s Settings) config.DNS {
	answers, _ := config.ParseAnswers(s.DNSAnswers)
	subnet, _ := clientSubnet(s.DNSClientSubnet)
	return config.DNS{
		Routing:      s.DNSRouting,
		FakeIP:       s.FakeIP,
		Hosts:        s.DNSHosts,
		Answers:      answers,
		NoCache:      !s.DNSCache,
		CacheSize:    uint32(s.DNSCacheSize),
		IgnoreTTL:    s.DNSIgnoreTTL,
		Optimistic:   s.DNSOptimistic,
		Timeout:      time.Duration(s.DNSTimeout) * time.Second,
		ClientSubnet: subnet,
		AddressFirst: s.AddressFirst,
		Object:       s.DNSObject,
	}
}

// withDefaultDNS gives a preset the DNS servers of the settings where it keeps «By default».
func withDefaultDNS(preset config.Preset, s Settings) config.Preset {
	if preset.DNS == "" {
		preset.DNS = s.DNSRemote
	}
	if preset.DirectDNS == "" {
		preset.DirectDNS = s.DNSDirect
	}
	return preset
}

// dnsChanged tells whether two settings resolve names differently.
func dnsChanged(a, b Settings) bool {
	return a.DNSRemote != b.DNSRemote || a.DNSDirect != b.DNSDirect || a.DNSRouting != b.DNSRouting || a.FakeIP != b.FakeIP ||
		a.DNSHosts != b.DNSHosts || a.DNSAnswers != b.DNSAnswers || a.DNSCache != b.DNSCache || a.DNSCacheSize != b.DNSCacheSize ||
		a.DNSIgnoreTTL != b.DNSIgnoreTTL || a.DNSOptimistic != b.DNSOptimistic || a.DNSTimeout != b.DNSTimeout ||
		a.DNSClientSubnet != b.DNSClientSubnet || a.DNSObject != b.DNSObject || a.AddressFirst != b.AddressFirst
}
