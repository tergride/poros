// Package config builds what the cores run: the sing-box config of a connection and, for servers only Xray runs,
// the Xray config behind a loopback bridge. It knows the server model, the routing presets and the user's switches,
// and nothing about processes, files or the network.
package config

import (
	"context"
	"errors"
	"net/netip"

	"PorosCore/profile"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/json/badoption"
)

// Tags of the config.
const (
	TagProxy  = "proxy"
	TagDirect = "direct"
	tagMixed  = "mixed-in"
	tagTun    = "tun-in"
	dnsRemote = "dns-remote"
	dnsDirect = "dns-direct"
)

// The TUN of Poros. Throne, which may run alongside, keeps 172.19.0.1/24.
const TunName = "poros-net"

var (
	tunAddress4 = netip.MustParsePrefix("172.19.1.1/24")
	tunAddress6 = netip.MustParsePrefix("fdfe:dcba:9877::1/96")
)

// The remote DNS: DoH by address, so that no lookup is needed to reach it.
const (
	remoteDNSServer = "8.8.8.8"
	remoteDNSPath   = "/dns-query"
)

// privateRanges stay out of the TUN unless a rule sends some of them elsewhere than direct: LAN, link-local and
// multicast traffic is no business of the server.
var privateRanges = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"), netip.MustParsePrefix("ff00::/8"),
}

var loopbackRanges = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")}

// Input is everything a connection is built from.
type Input struct {
	Server profile.Profile
	Preset Preset
	// Tun and the mixed inbound always exist; SystemProxy is the engine's business, not the config's.
	Tun bool
	// AllowLAN opens the mixed inbound to other devices.
	AllowLAN      bool
	MixedPort     uint16
	RealityOnXray bool
	// LogLevel is sing-box's; "" is info.
	LogLevel string

	// For a server on Xray: the bridge, the default interface Xray binds to, and the addresses of XrayHosts.
	Bridge    Bridge
	Interface string
	Resolved  map[string]netip.Addr
}

// Configs is a built connection. Xray is nil when sing-box runs the server itself.
type Configs struct {
	SingBox option.Options
	Xray    []byte
}

// SingBoxJSON writes the sing-box config the way the core reads it.
func (c Configs) SingBoxJSON() ([]byte, error) {
	return json.MarshalContext(context.Background(), c.SingBox)
}

// Build builds a connection. A server neither core can run fails with an error matching profile.ErrUnsupported.
func Build(in Input) (Configs, error) {
	if err := in.Server.Validate(); err != nil {
		return Configs{}, err
	}
	if err := in.Preset.Validate(); err != nil {
		return Configs{}, err
	}
	if in.MixedPort == 0 {
		return Configs{}, errors.New("the mixed port is missing")
	}
	var configs Configs
	var proxy option.Outbound
	if RunsOnXray(in.Server, in.RealityOnXray) {
		if in.Bridge.Port == 0 || in.Bridge.Username == "" || in.Bridge.Password == "" {
			return Configs{}, errors.New("a server on Xray needs the bridge")
		}
		xray, err := buildXray(in)
		if err != nil {
			return Configs{}, err
		}
		configs.Xray = xray
		proxy = option.Outbound{Type: C.TypeSOCKS, Tag: TagProxy, Options: &option.SOCKSOutboundOptions{
			ServerOptions: option.ServerOptions{Server: "127.0.0.1", ServerPort: in.Bridge.Port},
			Username:      in.Bridge.Username,
			Password:      in.Bridge.Password,
		}}
	} else {
		var err error
		if proxy, err = singBoxOutbound(in.Server, in.Preset.Hardened); err != nil {
			return Configs{}, err
		}
	}

	logLevel := in.LogLevel
	if logLevel == "" {
		logLevel = "info"
	}
	configs.SingBox = option.Options{
		Log:      &option.LogOptions{Level: logLevel},
		DNS:      buildDNS(in.Preset),
		Inbounds: buildInbounds(in),
		Outbounds: []option.Outbound{
			proxy,
			{Type: C.TypeDirect, Tag: TagDirect, Options: &option.DirectOutboundOptions{}},
		},
		Route: buildRoute(in),
	}
	return configs, nil
}

func buildInbounds(in Input) []option.Inbound {
	listen := netip.AddrFrom4([4]byte{127, 0, 0, 1})
	if in.AllowLAN {
		listen = netip.IPv4Unspecified()
	}
	inbounds := []option.Inbound{{Type: C.TypeMixed, Tag: tagMixed, Options: &option.HTTPMixedInboundOptions{
		ListenOptions: option.ListenOptions{Listen: (*badoption.Addr)(&listen), ListenPort: in.MixedPort},
	}}}
	if in.Tun {
		inbounds = append(inbounds, option.Inbound{Type: C.TypeTun, Tag: tagTun, Options: &option.TunInboundOptions{
			InterfaceName:       TunName,
			MTU:                 1500,
			Address:             badoption.Listable[netip.Prefix]{tunAddress4, tunAddress6},
			AutoRoute:           true,
			StrictRoute:         true,
			RouteExcludeAddress: tunExcluded(in.Preset),
		}})
	}
	return inbounds
}

// tunExcluded keeps broadcast and the private ranges out of the TUN, except those a rule sends elsewhere than
// direct: an excluded range never reaches the rules.
func tunExcluded(preset Preset) badoption.Listable[netip.Prefix] {
	excluded := badoption.Listable[netip.Prefix]{netip.MustParsePrefix("255.255.255.255/32")}
	var claimed []netip.Prefix
	for _, rule := range preset.Rules {
		if rule.Action == ActionHijackDNS || (rule.Action == ActionRoute && rule.Outbound == OutboundDirect) {
			continue
		}
		if rule.IPIsPrivate {
			return excluded
		}
		for _, cidr := range rule.IPCIDR {
			prefix, _ := parsePrefix(cidr)
			claimed = append(claimed, prefix)
		}
	}
	for _, private := range privateRanges {
		taken := false
		for _, prefix := range claimed {
			taken = taken || prefix.Overlaps(private)
		}
		if !taken {
			excluded = append(excluded, private)
		}
	}
	return excluded
}

func buildRoute(in Input) *option.RouteOptions {
	rules := []option.Rule{actionRule(option.RawDefaultRule{}, option.RuleAction{Action: C.RuleActionTypeSniff})}
	if in.AllowLAN {
		rules = append(rules, lanGuard())
	}
	for _, rule := range in.Preset.Rules {
		rules = append(rules, routeRule(rule))
	}
	final := TagProxy
	switch in.Preset.DefaultOutbound {
	case OutboundDirect:
		final = TagDirect
	case OutboundBlock:
		final = TagDirect
		rules = append(rules, actionRule(option.RawDefaultRule{}, option.RuleAction{Action: C.RuleActionTypeReject}))
	}
	return &option.RouteOptions{
		Rules:                 rules,
		Final:                 final,
		AutoDetectInterface:   in.Tun,
		DefaultDomainResolver: &option.DomainResolveOptions{Server: dnsDirect},
	}
}

// lanGuard keeps other devices on the mixed inbound away from this computer's own services.
func lanGuard() option.Rule {
	return option.Rule{Type: C.RuleTypeLogical, LogicalOptions: option.LogicalRule{
		RawLogicalRule: option.RawLogicalRule{Mode: C.LogicalTypeAnd, Rules: []option.Rule{
			{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: option.RawDefaultRule{
				Inbound:      badoption.Listable[string]{tagMixed},
				IPCIDR:       prefixables(loopbackRanges),
				Domain:       badoption.Listable[string]{"localhost"},
				DomainSuffix: badoption.Listable[string]{".localhost"},
			}}},
			{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: option.RawDefaultRule{
				SourceIPCIDR: prefixables(loopbackRanges),
				Invert:       true,
			}}},
		}},
		RuleAction: option.RuleAction{Action: C.RuleActionTypeReject},
	}}
}

func routeRule(rule Rule) option.Rule {
	raw := option.RawDefaultRule{
		Protocol:      rule.Protocol,
		Domain:        aceList(rule.Domain),
		DomainSuffix:  aceList(rule.DomainSuffix),
		DomainKeyword: rule.DomainKeyword,
		DomainRegex:   rule.DomainRegex,
		IPIsPrivate:   rule.IPIsPrivate,
		ProcessName:   rule.ProcessName,
		ProcessPath:   rule.ProcessPath,
	}
	for _, cidr := range rule.IPCIDR {
		prefix, _ := parsePrefix(cidr)
		raw.IPCIDR = append(raw.IPCIDR, (*badoption.Prefixable)(&prefix))
	}
	action := option.RuleAction{Action: rule.Action}
	if rule.Action == ActionRoute {
		action.RouteOptions.Outbound = rule.Outbound
	}
	return actionRule(raw, action)
}

func actionRule(raw option.RawDefaultRule, action option.RuleAction) option.Rule {
	return option.Rule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: raw, RuleAction: action}}
}

func prefixables(prefixes []netip.Prefix) badoption.Listable[*badoption.Prefixable] {
	result := make(badoption.Listable[*badoption.Prefixable], len(prefixes))
	for i := range prefixes {
		result[i] = (*badoption.Prefixable)(&prefixes[i])
	}
	return result
}

// buildDNS resolves the names of direct traffic with the system's DNS and the rest with DoH through the server, in
// the order of the rules, so that a name follows the same rule as its connection.
func buildDNS(preset Preset) *option.DNSOptions {
	servers := []option.DNSServerOptions{
		{Type: C.DNSTypeHTTPS, Tag: dnsRemote, Options: &option.RemoteHTTPSDNSServerOptions{
			RemoteTLSDNSServerOptions: option.RemoteTLSDNSServerOptions{RemoteDNSServerOptions: option.RemoteDNSServerOptions{
				RawLocalDNSServerOptions: option.RawLocalDNSServerOptions{DialerOptions: option.DialerOptions{Detour: TagProxy}},
				DNSServerAddressOptions:  option.DNSServerAddressOptions{Server: remoteDNSServer},
			}},
			Path: remoteDNSPath,
		}},
		{Type: C.DNSTypeLocal, Tag: dnsDirect, Options: &option.LocalDNSServerOptions{}},
	}
	var rules []option.DNSRule
	for _, rule := range preset.Rules {
		if rule.Action != ActionRoute {
			continue
		}
		raw := option.RawDefaultDNSRule{
			Domain:        aceList(rule.Domain),
			DomainSuffix:  aceList(rule.DomainSuffix),
			DomainKeyword: rule.DomainKeyword,
			DomainRegex:   rule.DomainRegex,
			ProcessName:   rule.ProcessName,
			ProcessPath:   rule.ProcessPath,
		}
		if len(raw.Domain)+len(raw.DomainSuffix)+len(raw.DomainKeyword)+len(raw.DomainRegex)+len(raw.ProcessName)+len(raw.ProcessPath) == 0 {
			continue
		}
		server := dnsRemote
		if rule.Outbound == OutboundDirect {
			server = dnsDirect
		}
		rules = append(rules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultDNSRule{
			RawDefaultDNSRule: raw,
			DNSRuleAction:     option.DNSRuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.DNSRouteActionOptions{Server: server}},
		}})
	}
	final := dnsRemote
	if preset.DefaultOutbound == OutboundDirect {
		final = dnsDirect
	}
	return &option.DNSOptions{RawDNSOptions: option.RawDNSOptions{Servers: servers, Rules: rules, Final: final}}
}
