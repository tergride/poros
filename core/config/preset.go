package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// PresetKind marks a preset file, the export of a preset, and the data of a poros://route/ link.
const PresetKind = "poros-route-profile"

// Outbounds a preset sends traffic to.
const (
	OutboundProxy  = "proxy"
	OutboundDirect = "direct"
	// OutboundBlock is a default outbound only: it drops what no rule took.
	OutboundBlock = "block"
)

// EndpointTag is the outbound of an endpoint of a preset: the i-th of Preset.Endpoints, from 0.
func EndpointTag(i int) string {
	return endpointPrefix + strconv.Itoa(i+1)
}

const endpointPrefix = "endpoint-"

// endpointIndex gives the place in Preset.Endpoints of the endpoint an outbound names.
func endpointIndex(outbound string) (int, bool) {
	text, found := strings.CutPrefix(outbound, endpointPrefix)
	n, err := strconv.Atoi(text)
	if !found || err != nil || n < 1 || strconv.Itoa(n) != text {
		return 0, false
	}
	return n - 1, true
}

// Rule actions.
const (
	ActionRoute     = "route"
	ActionReject    = "reject"
	ActionHijackDNS = "hijack-dns"
)

// Preset picks the outbound of every connection: the first rule that matches decides, the default outbound takes
// the rest.
type Preset struct {
	Name string `json:"name"`
	// Hardened splits the TLS handshake of the server connection to slip past DPI; servers on REALITY, QUIC and
	// Xray connect as usual.
	Hardened        bool   `json:"hardened,omitempty"`
	DefaultOutbound string `json:"default_outbound"`
	// DNS is the remote DNS server, reached through the server: https://host[:port][/path], tls://host[:port] or
	// a plain address over UDP. Empty is Google's DoH.
	DNS string `json:"dns,omitempty"`
	// DirectDNS is the DNS server of direct traffic, reached past the server and written as DNS is. Empty and
	// SystemDNS are the system's DNS.
	DirectDNS string `json:"direct_dns,omitempty"`
	Rules     []Rule `json:"rules"`
	// Endpoints are the IDs of the OpenVPN and OpenConnect profiles that connect alongside the server: the
	// connection gives them as Input.Endpoints, in this order, and a rule reaches one through its EndpointTag.
	// They are this computer's profiles, so no preset file has them.
	Endpoints []string `json:"-"`
}

// Rule matches a connection when every condition it has matches, and a condition matches by any of its values.
// Invert takes what the conditions do not match.
type Rule struct {
	Name string `json:"name,omitempty"`
	// Action is route, reject or hijack-dns; Outbound is proxy, direct, warp or an EndpointTag and goes with route.
	Action        string   `json:"action"`
	Outbound      string   `json:"outbound,omitempty"`
	Invert        bool     `json:"invert,omitempty"`
	Protocol      []string `json:"protocol,omitempty"`
	Network       []string `json:"network,omitempty"`
	IPVersion     int      `json:"ip_version,omitempty"`
	Domain        []string `json:"domain,omitempty"`
	DomainSuffix  []string `json:"domain_suffix,omitempty"`
	DomainKeyword []string `json:"domain_keyword,omitempty"`
	DomainRegex   []string `json:"domain_regex,omitempty"`
	IPCIDR        []string `json:"ip_cidr,omitempty"`
	IPIsPrivate   bool     `json:"ip_is_private,omitempty"`
	SourceIPCIDR  []string `json:"source_ip_cidr,omitempty"`
	// Port and PortRange (first:last) are one condition: a port matches either.
	Port             []uint16 `json:"port,omitempty"`
	PortRange        []string `json:"port_range,omitempty"`
	ProcessName      []string `json:"process_name,omitempty"`
	ProcessPath      []string `json:"process_path,omitempty"`
	ProcessPathRegex []string `json:"process_path_regex,omitempty"`
}

type presetFile struct {
	Kind    string `json:"kind"`
	Version int    `json:"v"`
	Preset
}

// ParsePreset reads a preset file. Unknown keys are refused: a rule that lost a condition would match more.
func ParsePreset(data []byte) (Preset, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var file presetFile
	if err := decoder.Decode(&file); err != nil {
		return Preset{}, fmt.Errorf("read the preset: %w", err)
	}
	if file.Kind != PresetKind || file.Version != 1 {
		return Preset{}, fmt.Errorf("not a %s, version 1", PresetKind)
	}
	if err := file.Preset.Validate(); err != nil {
		return Preset{}, err
	}
	return file.Preset, nil
}

// withoutEndpoints sends what the preset sends to its endpoints to the server instead.
func (p Preset) withoutEndpoints() Preset {
	p.Rules = slices.Clone(p.Rules)
	for i := range p.Rules {
		if _, ok := endpointIndex(p.Rules[i].Outbound); ok {
			p.Rules[i].Outbound = OutboundProxy
		}
	}
	p.Endpoints = nil
	return p
}

// withoutWARP sends what the preset sends through WARP to the server instead.
func (p Preset) withoutWARP() Preset {
	p.Rules = slices.Clone(p.Rules)
	for i := range p.Rules {
		if p.Rules[i].Outbound == OutboundWARP {
			p.Rules[i].Outbound = OutboundProxy
		}
	}
	return p
}

// directAddresses tells a rule that sends addresses, and nothing else of a connection, direct.
func (r Rule) directAddresses() bool {
	return r.Action == ActionRoute && r.Outbound == OutboundDirect && !r.Invert && len(r.IPCIDR) > 0 && len(r.conditions()) == 1
}

func (p Preset) usesWARP() bool {
	return slices.ContainsFunc(p.Rules, func(r Rule) bool { return r.Outbound == OutboundWARP })
}

// Validate checks what the config builder relies on.
func (p Preset) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("the preset has no name")
	}
	if !slices.Contains([]string{OutboundProxy, OutboundDirect, OutboundBlock}, p.DefaultOutbound) {
		return fmt.Errorf("unknown default outbound %q", p.DefaultOutbound)
	}
	for i, rule := range p.Rules {
		if err := rule.validate(); err != nil {
			return fmt.Errorf("rule %d %q: %w", i+1, rule.Name, err)
		}
		if n, ok := endpointIndex(rule.Outbound); ok && n >= len(p.Endpoints) {
			return fmt.Errorf("rule %d %q: the preset has no endpoint %d", i+1, rule.Name, n+1)
		}
	}
	if p.DNS != "" {
		if _, err := remoteDNS(p.DNS); err != nil {
			return err
		}
	}
	if p.DirectDNS != "" && p.DirectDNS != SystemDNS {
		if _, err := directDNS(p.DirectDNS); err != nil {
			return err
		}
	}
	return nil
}

func (r Rule) validate() error {
	switch r.Action {
	case ActionRoute:
		if _, endpoint := endpointIndex(r.Outbound); r.Outbound != OutboundProxy && r.Outbound != OutboundDirect && r.Outbound != OutboundWARP && !endpoint {
			return fmt.Errorf("unknown outbound %q", r.Outbound)
		}
	case ActionReject, ActionHijackDNS:
		if r.Outbound != "" {
			return fmt.Errorf("%s takes no outbound", r.Action)
		}
	default:
		return fmt.Errorf("unknown action %q", r.Action)
	}
	if len(r.conditions()) == 0 {
		// A rule without conditions would take everything after it.
		return errors.New("the rule has no condition")
	}
	lists := [][]string{r.Protocol, r.Network, r.Domain, r.DomainSuffix, r.DomainKeyword, r.DomainRegex, r.PortRange, r.ProcessName, r.ProcessPath, r.ProcessPathRegex}
	for _, list := range lists {
		if slices.ContainsFunc(list, func(item string) bool { return strings.TrimSpace(item) == "" }) {
			return errors.New("the rule has an empty value")
		}
	}
	for _, network := range r.Network {
		if network != "tcp" && network != "udp" {
			return fmt.Errorf("unknown network %q", network)
		}
	}
	if r.IPVersion != 0 && r.IPVersion != 4 && r.IPVersion != 6 {
		return fmt.Errorf("unknown IP version %d", r.IPVersion)
	}
	for _, cidr := range slices.Concat(r.IPCIDR, r.SourceIPCIDR) {
		if _, err := parsePrefix(cidr); err != nil {
			return err
		}
	}
	if slices.Contains(r.Port, 0) {
		return errors.New("port 0 matches nothing")
	}
	for _, text := range r.PortRange {
		if _, _, ok := ParsePortRange(text); !ok {
			return fmt.Errorf("%q is no port range", text)
		}
	}
	for _, pattern := range slices.Concat(r.DomainRegex, r.ProcessPathRegex) {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("%q is no regular expression: %w", pattern, err)
		}
	}
	return nil
}

// condition fills one condition of a rule into a sing-box rule: the fields that match together.
type condition func(raw *option.RawDefaultRule)

// conditions gives the conditions the rule has, in a fixed order.
func (r Rule) conditions() []condition {
	var list []condition
	add := func(set bool, fill condition) {
		if set {
			list = append(list, fill)
		}
	}
	add(len(r.Protocol) > 0, func(raw *option.RawDefaultRule) { raw.Protocol = r.Protocol })
	add(len(r.Network) > 0, func(raw *option.RawDefaultRule) { raw.Network = r.Network })
	add(r.IPVersion != 0, func(raw *option.RawDefaultRule) { raw.IPVersion = r.IPVersion })
	add(len(r.Domain) > 0, func(raw *option.RawDefaultRule) { raw.Domain = aceList(r.Domain) })
	add(len(r.DomainSuffix) > 0, func(raw *option.RawDefaultRule) { raw.DomainSuffix = aceList(r.DomainSuffix) })
	add(len(r.DomainKeyword) > 0, func(raw *option.RawDefaultRule) { raw.DomainKeyword = r.DomainKeyword })
	add(len(r.DomainRegex) > 0, func(raw *option.RawDefaultRule) { raw.DomainRegex = r.DomainRegex })
	add(len(r.IPCIDR) > 0, func(raw *option.RawDefaultRule) { raw.IPCIDR = prefixList(r.IPCIDR) })
	add(r.IPIsPrivate, func(raw *option.RawDefaultRule) { raw.IPIsPrivate = true })
	add(len(r.SourceIPCIDR) > 0, func(raw *option.RawDefaultRule) { raw.SourceIPCIDR = prefixList(r.SourceIPCIDR) })
	add(len(r.Port)+len(r.PortRange) > 0, func(raw *option.RawDefaultRule) { raw.Port, raw.PortRange = r.Port, r.PortRange })
	add(len(r.ProcessName) > 0, func(raw *option.RawDefaultRule) { raw.ProcessName = r.ProcessName })
	add(len(r.ProcessPath) > 0, func(raw *option.RawDefaultRule) { raw.ProcessPath = r.ProcessPath })
	add(len(r.ProcessPathRegex) > 0, func(raw *option.RawDefaultRule) { raw.ProcessPathRegex = r.ProcessPathRegex })
	return list
}

// destinations counts the conditions on the destination address. sing-box matches a default rule when any of these
// does, so a rule with two of them is built of separate rules that must all match.
func (r Rule) destinations() int {
	n := 0
	for _, set := range []bool{len(r.Domain) > 0, len(r.DomainSuffix) > 0, len(r.DomainKeyword) > 0, len(r.DomainRegex) > 0, len(r.IPCIDR) > 0, r.IPIsPrivate} {
		if set {
			n++
		}
	}
	return n
}

// ParsePortRange reads a port range of sing-box, first:last.
func ParsePortRange(text string) (first, last uint16, ok bool) {
	a, b, found := strings.Cut(text, ":")
	from, errA := strconv.ParseUint(a, 10, 16)
	to, errB := strconv.ParseUint(b, 10, 16)
	if !found || errA != nil || errB != nil || from == 0 || from > to {
		return 0, 0, false
	}
	return uint16(from), uint16(to), true
}

func prefixList(cidrs []string) badoption.Listable[*badoption.Prefixable] {
	var list badoption.Listable[*badoption.Prefixable]
	for _, cidr := range cidrs {
		prefix, _ := parsePrefix(cidr)
		list = append(list, (*badoption.Prefixable)(&prefix))
	}
	return list
}

// parsePrefix reads a CIDR or a bare address, which stands for itself.
func parsePrefix(text string) (netip.Prefix, error) {
	if prefix, err := netip.ParsePrefix(text); err == nil {
		return prefix.Masked(), nil
	}
	addr, err := netip.ParseAddr(text)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%q is neither an address nor a subnet", text)
	}
	return netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen()), nil
}
