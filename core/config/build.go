// Package config builds what the cores run: the sing-box config of a connection and, for servers only Xray runs,
// the Xray config behind a loopback bridge. It knows the server model, the routing presets and the user's switches,
// and nothing about processes, files or the network.
package config

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"

	"PorosCore/profile"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/json/badoption"
)

// Tags of the config.
const (
	TagProxy     = "proxy"
	TagDirect    = "direct"
	tagMixed     = "mixed-in"
	tagTun       = "tun-in"
	TagDNSRemote = "dns-remote"
	TagDNSDirect = "dns-direct"
	// tagDNSSystem resolves the name of a direct DNS server of the user's.
	tagDNSSystem = "dns-system"
	tagDNSHosts  = "dns-hosts"
	tagDNSFake   = "dns-fake"
)

// The ranges of FakeIP. IPv6 takes a documentation range: the TUN keeps fc00::/7 out.
var (
	fakeIPRange4 = netip.MustParsePrefix("198.18.0.0/15")
	fakeIPRange6 = netip.MustParsePrefix("2001:db8::/32")
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
// multicast traffic is no business of the server. The unicast ones are those of the networks behind endpoints too.
var privateRanges = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"), netip.MustParsePrefix("ff00::/8"),
}

func privateUnicast(prefix netip.Prefix) bool {
	return prefix.Addr().IsPrivate()
}

var loopbackRanges = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")}

// Input is everything a connection is built from.
type Input struct {
	// Refuse builds the connection of the kill switch without a server: what the preset sends to the server, its
	// DNS included, is refused, and the rest goes as the preset says. Server is not used.
	Refuse bool
	Server profile.Profile
	// Links are the servers of a chain, in its order.
	Links  []profile.Profile
	Preset Preset
	// Endpoints are the profiles of Preset.Endpoints, in its order. A connection that refuses has none: what the
	// preset sends to them is refused with the rest of the server's traffic.
	Endpoints []profile.Profile
	// WARP is the exit of the rules to OutboundWARP, nil while WARP is off: their traffic goes through the server
	// then. A connection that refuses has none either.
	WARP *WARP
	// Tun and the mixed inbound always exist; SystemProxy is the engine's business, not the config's.
	Tun bool
	// AllowLAN opens the mixed inbound to other devices.
	AllowLAN bool
	// DisableIPv6 answers names with IPv4 only and refuses IPv6 connections other than DNS. Names of connections
	// are resolved here, so that the server too reaches sites over IPv4. The TUN still takes IPv6, so that it never
	// goes around the tunnel.
	DisableIPv6   bool
	MixedPort     uint16
	RealityOnXray bool
	// LogLevel is sing-box's; "" is info.
	LogLevel string
	// DNS is how names resolve. A connection that refuses resolves them as their traffic goes even when the
	// settings send all of them through the server: else no direct site would open.
	DNS DNS

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
	if !in.Refuse {
		if err := validateServer(in.Server, in.Links); err != nil {
			return Configs{}, err
		}
	}
	if err := in.Preset.Validate(); err != nil {
		return Configs{}, err
	}
	if in.MixedPort == 0 {
		return Configs{}, errors.New("the mixed port is missing")
	}
	preset, profiles := in.Preset, in.Endpoints
	var endpoints []option.Endpoint
	if in.Refuse {
		preset, profiles = preset.withoutEndpoints(), nil
	} else {
		var err error
		if endpoints, err = endpointParts(preset, profiles); err != nil {
			return Configs{}, err
		}
	}
	if in.Refuse || in.WARP == nil {
		preset = preset.withoutWARP()
	} else if preset.usesWARP() {
		warp, err := warpEndpoint(in.WARP)
		if err != nil {
			return Configs{}, fmt.Errorf("WARP: %w", err)
		}
		endpoints = append(endpoints, warp)
	}
	var configs Configs
	var proxy boxServer
	switch {
	case in.Refuse:
		proxy.outbounds = []option.Outbound{{Type: C.TypeBlock, Tag: TagProxy, Options: &option.StubOptions{}}}
	default:
		var bridge *option.Outbound
		if entry := Entry(in.Server, in.Links); RunsOnXray(entry, in.RealityOnXray) {
			if in.Bridge.Port == 0 || in.Bridge.Username == "" || in.Bridge.Password == "" {
				return Configs{}, errors.New("a server on Xray needs the bridge")
			}
			tag := entryTag(in.Server, TagProxy)
			xray, err := buildXray(in, entry, tag)
			if err != nil {
				return Configs{}, err
			}
			configs.Xray = xray
			b := bridgeOutbound(tag, in.Bridge)
			bridge = &b
		}
		var err error
		if proxy, err = serverParts(in.Server, in.Links, TagProxy, bridge, in.Preset.Hardened); err != nil {
			return Configs{}, err
		}
	}

	logLevel := in.LogLevel
	if logLevel == "" {
		logLevel = "info"
	}
	if in.Refuse && in.DNS.Routing == RoutingProxy {
		in.DNS.Routing = RoutingAuto
	}
	dns, err := buildDNS(in, preset, profiles)
	if err != nil {
		return Configs{}, err
	}
	direct := &option.DirectOutboundOptions{}
	// All names through the server: so are those of direct connections the local proxy is asked for by name.
	if in.DNS.Routing == RoutingProxy && in.DNS.Object == "" {
		direct.DomainResolver = &option.DomainResolveOptions{Server: TagDNSRemote, Strategy: addressStrategy(in)}
	}
	configs.SingBox = option.Options{
		Log:       &option.LogOptions{Level: logLevel},
		DNS:       dns,
		Inbounds:  buildInbounds(in, preset),
		Outbounds: append(proxy.outbounds, option.Outbound{Type: C.TypeDirect, Tag: TagDirect, Options: direct}),
		Endpoints: slices.Concat(proxy.endpoints, endpoints),
		Route:     buildRoute(in, preset),
	}
	return configs, nil
}

// addressStrategy is which address of a name a connection takes: IPv4 alone without IPv6, else the family the
// settings put first.
func addressStrategy(in Input) option.DomainStrategy {
	switch {
	case in.DisableIPv6:
		return option.DomainStrategy(C.DomainStrategyIPv4Only)
	case in.DNS.AddressFirst == AddressIPv4:
		return option.DomainStrategy(C.DomainStrategyPreferIPv4)
	case in.DNS.AddressFirst == AddressIPv6:
		return option.DomainStrategy(C.DomainStrategyPreferIPv6)
	}
	return option.DomainStrategy(C.DomainStrategyAsIS)
}

// endpointParts builds the endpoints of a preset. They dial their servers directly, not through the server.
func endpointParts(preset Preset, profiles []profile.Profile) ([]option.Endpoint, error) {
	if len(profiles) != len(preset.Endpoints) {
		return nil, errors.New("the endpoints of the preset are missing")
	}
	var endpoints []option.Endpoint
	for i, p := range profiles {
		if p.Type != profile.TypeOpenVPN && p.Type != profile.TypeOpenConnect {
			return nil, fmt.Errorf("endpoint %d: %s is no OpenVPN or OpenConnect profile", i+1, p.Type)
		}
		if err := p.Validate(); err != nil {
			return nil, fmt.Errorf("endpoint %d: %w", i+1, err)
		}
		parts, err := singBoxServer(p, EndpointTag(i), false)
		if err != nil {
			return nil, fmt.Errorf("endpoint %d: %w", i+1, err)
		}
		endpoints = append(endpoints, parts.endpoints...)
	}
	return endpoints, nil
}

// bridgeOutbound reaches a server on Xray through the bridge Xray listens on.
func bridgeOutbound(tag string, bridge Bridge) option.Outbound {
	return option.Outbound{Type: C.TypeSOCKS, Tag: tag, Options: &option.SOCKSOutboundOptions{
		ServerOptions: option.ServerOptions{Server: "127.0.0.1", ServerPort: bridge.Port},
		Username:      bridge.Username,
		Password:      bridge.Password,
	}}
}

func buildInbounds(in Input, preset Preset) []option.Inbound {
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
			RouteExcludeAddress: tunExcluded(preset),
		}})
	}
	return inbounds
}

// tunExcluded keeps broadcast and the private ranges out of the TUN, except those a rule sends elsewhere than
// direct: an excluded range never reaches the rules. The networks of an endpoint are known only once it is up, and
// are mostly private ones: with endpoints, the private unicast ranges stay in the TUN, and the rule of local networks
// sends what no endpoint takes direct.
func tunExcluded(preset Preset) badoption.Listable[netip.Prefix] {
	excluded := badoption.Listable[netip.Prefix]{netip.MustParsePrefix("255.255.255.255/32")}
	var claimed []netip.Prefix
	for _, rule := range preset.Rules {
		if rule.Action == ActionHijackDNS || (rule.Action == ActionRoute && rule.Outbound == OutboundDirect) {
			continue
		}
		// An inverted rule on addresses takes every address but its own, the private ones among them.
		if (rule.IPIsPrivate && !rule.Invert) || (rule.Invert && len(rule.IPCIDR) > 0) {
			return excluded
		}
		if rule.Invert {
			continue
		}
		for _, cidr := range rule.IPCIDR {
			prefix, _ := parsePrefix(cidr)
			claimed = append(claimed, prefix)
		}
	}
	for _, private := range privateRanges {
		taken := len(preset.Endpoints) > 0 && privateUnicast(private)
		for _, prefix := range claimed {
			taken = taken || prefix.Overlaps(private)
		}
		if !taken {
			excluded = append(excluded, private)
		}
	}
	return excluded
}

func buildRoute(in Input, preset Preset) *option.RouteOptions {
	rules := []option.Rule{actionRule(option.RawDefaultRule{}, option.RuleAction{Action: C.RuleActionTypeSniff})}
	if in.DisableIPv6 {
		rules = append(rules, ipv6Block(), actionRule(option.RawDefaultRule{}, option.RuleAction{
			Action:         C.RuleActionTypeResolve,
			ResolveOptions: option.RouteActionResolve{Strategy: option.DomainStrategy(C.DomainStrategyIPv4Only)},
		}))
	}
	if in.AllowLAN {
		rules = append(rules, lanGuard())
	}
	// The networks of the endpoints come right after the DNS queries, before the user's rules: an endpoint is the one
	// way to them.
	head := dnsHead(preset.Rules)
	for _, rule := range preset.Rules[:head] {
		rules = append(rules, routeRule(rule))
	}
	for i := range preset.Endpoints {
		tag := EndpointTag(i)
		rules = append(rules, actionRule(option.RawDefaultRule{PreferredBy: badoption.Listable[string]{tag}},
			option.RuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.RouteActionOptions{Outbound: tag}}))
	}
	for _, rule := range preset.Rules[head:] {
		rules = append(rules, routeRule(rule))
	}
	final := TagProxy
	switch preset.DefaultOutbound {
	case OutboundDirect:
		final = TagDirect
	case OutboundBlock:
		final = TagDirect
		rules = append(rules, actionRule(option.RawDefaultRule{}, option.RuleAction{Action: C.RuleActionTypeReject}))
	}
	resolver := &option.DomainResolveOptions{Server: TagDNSDirect, Strategy: addressStrategy(in)}
	return &option.RouteOptions{
		Rules:                 rules,
		Final:                 final,
		AutoDetectInterface:   in.Tun,
		DefaultDomainResolver: resolver,
	}
}

// ipv6Block refuses IPv6 connections at once, so that programs fall back to IPv4; DNS over IPv6 is still answered.
func ipv6Block() option.Rule {
	return option.Rule{Type: C.RuleTypeLogical, LogicalOptions: option.LogicalRule{
		RawLogicalRule: option.RawLogicalRule{Mode: C.LogicalTypeAnd, Rules: []option.Rule{
			{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: option.RawDefaultRule{IPVersion: 6}}},
			{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: option.RawDefaultRule{
				Protocol: badoption.Listable[string]{C.ProtocolDNS},
				Invert:   true,
			}}},
		}},
		RuleAction: option.RuleAction{Action: C.RuleActionTypeReject},
	}}
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

// dnsHead counts the rules of DNS queries a preset starts with.
func dnsHead(rules []Rule) int {
	n := 0
	for n < len(rules) && rules[n].Action == ActionHijackDNS {
		n++
	}
	return n
}

func routeRule(rule Rule) option.Rule {
	action := option.RuleAction{Action: rule.Action}
	if rule.Action == ActionRoute {
		action.RouteOptions.Outbound = rule.Outbound
	}
	conditions := rule.conditions()
	if rule.destinations() <= 1 {
		var raw option.RawDefaultRule
		for _, fill := range conditions {
			fill(&raw)
		}
		raw.Invert = rule.Invert
		return actionRule(raw, action)
	}
	parts := make([]option.Rule, len(conditions))
	for i, fill := range conditions {
		var raw option.RawDefaultRule
		fill(&raw)
		parts[i] = option.Rule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: raw}}
	}
	return option.Rule{Type: C.RuleTypeLogical, LogicalOptions: option.LogicalRule{
		RawLogicalRule: option.RawLogicalRule{Mode: C.LogicalTypeAnd, Rules: parts, Invert: rule.Invert},
		RuleAction:     action,
	}}
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

// buildDNS resolves names the way the settings say. As their traffic goes: the names of direct traffic with the
// direct DNS, the system's unless the preset has its own, the names an endpoint's rules send to it with the DNS it was
// given, and the rest with the remote DNS through the server, in the order of the rules, so that a name follows the
// same rule as its connection. Before any rule, ready answers, the hosts file, the domains of the endpoints and
// FakeIP answer what they have. An own dns object of the settings replaces all of it.
func buildDNS(in Input, preset Preset, endpoints []profile.Profile) (*option.DNSOptions, error) {
	if in.DNS.Object != "" {
		options, err := parseDNSObject(in.DNS.Object)
		if err != nil {
			return nil, err
		}
		// The addresses of servers resolve with dns-direct: the system's DNS unless the object has its own.
		if !slices.ContainsFunc(options.Servers, func(s option.DNSServerOptions) bool { return s.Tag == TagDNSDirect }) {
			options.Servers = append(options.Servers, option.DNSServerOptions{Type: C.DNSTypeLocal, Tag: TagDNSDirect, Options: &option.LocalDNSServerOptions{}})
		}
		return options, nil
	}
	text := preset.DNS
	if text == "" {
		text = "https://" + remoteDNSServer + remoteDNSPath
	}
	// Validate has read both already.
	remote, _ := remoteDNS(text)
	servers := []option.DNSServerOptions{remote}
	if preset.DirectDNS == "" || preset.DirectDNS == SystemDNS {
		servers = append(servers, option.DNSServerOptions{Type: C.DNSTypeLocal, Tag: TagDNSDirect, Options: &option.LocalDNSServerOptions{}})
	} else {
		direct, _ := directDNS(preset.DirectDNS)
		servers = append(servers, direct, option.DNSServerOptions{Type: C.DNSTypeLocal, Tag: tagDNSSystem, Options: &option.LocalDNSServerOptions{}})
	}
	addresses := badoption.Listable[option.DNSQueryType]{option.DNSQueryType(1), option.DNSQueryType(28)}
	route := func(server string) option.DNSRuleAction {
		return option.DNSRuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.DNSRouteActionOptions{Server: server}}
	}
	rules := answerRules(in.DNS.Answers)
	if in.DNS.Hosts {
		// The hosts server answers no name it lacks, so only those it has go to it.
		servers = append(servers, option.DNSServerOptions{Type: C.DNSTypeHosts, Tag: tagDNSHosts, Options: &option.HostsDNSServerOptions{}})
		rules = append(rules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultDNSRule{
			RawDefaultDNSRule: option.RawDefaultDNSRule{PreferredBy: badoption.Listable[string]{tagDNSHosts}, QueryType: addresses},
			DNSRuleAction:     route(tagDNSHosts),
		}})
	}
	for i, p := range endpoints {
		tag := dnsEndpointTag(i)
		servers = append(servers, endpointDNS(p, i))
		rules = append(rules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultDNSRule{
			RawDefaultDNSRule: option.RawDefaultDNSRule{PreferredBy: badoption.Listable[string]{tag}},
			DNSRuleAction:     route(tag),
		}})
	}
	// Only the queries of programs get fake addresses: sing-box resolves its own connections with real ones.
	if in.DNS.FakeIP && in.Tun {
		fake := &option.FakeIPDNSServerOptions{Inet4Range: (*badoption.Prefix)(&fakeIPRange4)}
		if !in.DisableIPv6 {
			fake.Inet6Range = (*badoption.Prefix)(&fakeIPRange6)
		}
		servers = append(servers, option.DNSServerOptions{Type: C.DNSTypeFakeIP, Tag: tagDNSFake, Options: fake})
		rules = append(rules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultDNSRule{
			RawDefaultDNSRule: option.RawDefaultDNSRule{QueryType: addresses},
			DNSRuleAction:     route(tagDNSFake),
		}})
	}
	final := TagDNSRemote
	switch in.DNS.Routing {
	case RoutingProxy:
	case RoutingDirect:
		final = TagDNSDirect
	default:
		for _, rule := range preset.Rules {
			if rule.Action != ActionRoute {
				continue
			}
			server := TagDNSRemote
			if rule.Outbound == OutboundDirect {
				server = TagDNSDirect
			} else if n, ok := endpointIndex(rule.Outbound); ok {
				server = dnsEndpointTag(n)
			}
			if dns, ok := dnsRule(rule, route(server)); ok {
				rules = append(rules, dns)
			}
		}
		if preset.DefaultOutbound == OutboundDirect {
			final = TagDNSDirect
		}
	}
	options := &option.DNSOptions{RawDNSOptions: option.RawDNSOptions{Servers: servers, Rules: rules, Final: final}}
	client := &options.DNSClientOptions
	client.Strategy = addressStrategy(in)
	client.DisableCache, client.CacheCapacity, client.DisableExpire = in.DNS.NoCache, in.DNS.CacheSize, in.DNS.IgnoreTTL
	if in.DNS.Optimistic && !in.DNS.NoCache && !in.DNS.IgnoreTTL {
		client.Optimistic = &option.OptimisticDNSOptions{Enabled: true}
	}
	client.Timeout = badoption.Duration(in.DNS.Timeout)
	if in.DNS.ClientSubnet.IsValid() {
		subnet := in.DNS.ClientSubnet
		client.ClientSubnet = (*badoption.Prefixable)(&subnet)
	}
	return options, nil
}

func dnsEndpointTag(i int) string {
	return "dns-" + EndpointTag(i)
}

// endpointDNS is the DNS an endpoint was given by its server, reached through the endpoint. It answers the names of
// the endpoint's domains and, sent there by a rule, any other name.
func endpointDNS(p profile.Profile, i int) option.DNSServerOptions {
	if p.Type == profile.TypeOpenConnect {
		return option.DNSServerOptions{Type: C.DNSTypeOpenConnect, Tag: dnsEndpointTag(i), Options: &option.OpenConnectDNSServerOptions{
			Endpoint: EndpointTag(i), AcceptDefaultResolvers: true,
		}}
	}
	return option.DNSServerOptions{Type: C.DNSTypeOpenVPN, Tag: dnsEndpointTag(i), Options: &option.OpenVPNDNSServerOptions{
		Endpoint: EndpointTag(i), AcceptDefaultResolvers: true,
	}}
}

// dnsRule is the DNS rule of a rule on names and programs: a query has neither an address, a port nor a protocol of
// the connection to come, so a rule with one of them has no DNS rule.
func dnsRule(rule Rule, action option.DNSRuleAction) (option.DNSRule, bool) {
	if len(rule.Protocol)+len(rule.Network)+len(rule.IPCIDR)+len(rule.SourceIPCIDR)+len(rule.Port)+len(rule.PortRange) > 0 ||
		rule.IPVersion != 0 || rule.IPIsPrivate {
		return option.DNSRule{}, false
	}
	var fills []func(raw *option.RawDefaultDNSRule)
	add := func(set bool, fill func(raw *option.RawDefaultDNSRule)) {
		if set {
			fills = append(fills, fill)
		}
	}
	add(len(rule.Domain) > 0, func(raw *option.RawDefaultDNSRule) { raw.Domain = aceList(rule.Domain) })
	add(len(rule.DomainSuffix) > 0, func(raw *option.RawDefaultDNSRule) { raw.DomainSuffix = aceList(rule.DomainSuffix) })
	add(len(rule.DomainKeyword) > 0, func(raw *option.RawDefaultDNSRule) { raw.DomainKeyword = rule.DomainKeyword })
	add(len(rule.DomainRegex) > 0, func(raw *option.RawDefaultDNSRule) { raw.DomainRegex = rule.DomainRegex })
	add(len(rule.ProcessName) > 0, func(raw *option.RawDefaultDNSRule) { raw.ProcessName = rule.ProcessName })
	add(len(rule.ProcessPath) > 0, func(raw *option.RawDefaultDNSRule) { raw.ProcessPath = rule.ProcessPath })
	add(len(rule.ProcessPathRegex) > 0, func(raw *option.RawDefaultDNSRule) { raw.ProcessPathRegex = rule.ProcessPathRegex })
	if len(fills) == 0 {
		return option.DNSRule{}, false
	}
	if rule.destinations() <= 1 {
		var raw option.RawDefaultDNSRule
		for _, fill := range fills {
			fill(&raw)
		}
		raw.Invert = rule.Invert
		return option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultDNSRule{RawDefaultDNSRule: raw, DNSRuleAction: action}}, true
	}
	parts := make([]option.DNSRule, len(fills))
	for i, fill := range fills {
		var raw option.RawDefaultDNSRule
		fill(&raw)
		parts[i] = option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultDNSRule{RawDefaultDNSRule: raw}}
	}
	return option.DNSRule{Type: C.RuleTypeLogical, LogicalOptions: option.LogicalDNSRule{
		RawLogicalDNSRule: option.RawLogicalDNSRule{Mode: C.LogicalTypeAnd, Rules: parts, Invert: rule.Invert},
		DNSRuleAction:     action,
	}}, true
}
