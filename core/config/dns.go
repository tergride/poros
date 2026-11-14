package config

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/json/badoption"
)

// remoteDNS reads the remote DNS server of a preset into the dns-remote server, dialed through the server. A name
// in its address is resolved with the DNS of direct traffic: the remote one cannot resolve its own address.
func remoteDNS(text string) (option.DNSServerOptions, error) {
	return dnsServer(text, TagDNSRemote, option.DialerOptions{Detour: TagProxy}, TagDNSDirect)
}

// directDNS reads the direct DNS server of a preset into the dns-direct server, dialed past the server. A name in
// its address is resolved with the system's DNS.
func directDNS(text string) (option.DNSServerOptions, error) {
	return dnsServer(text, TagDNSDirect, option.DialerOptions{}, tagDNSSystem)
}

func dnsServer(text, tag string, dialer option.DialerOptions, resolver string) (option.DNSServerOptions, error) {
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
	if net.ParseIP(host) == nil {
		dialer.DomainResolver = &option.DomainResolveOptions{Server: resolver}
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
		return option.DNSServerOptions{Type: C.DNSTypeHTTPS, Tag: tag, Options: &option.RemoteHTTPSDNSServerOptions{
			RemoteTLSDNSServerOptions: option.RemoteTLSDNSServerOptions{RemoteDNSServerOptions: remote},
			Path:                      path,
		}}, nil
	case "tls":
		if parsed.Path != "" {
			return option.DNSServerOptions{}, invalid
		}
		return option.DNSServerOptions{Type: C.DNSTypeTLS, Tag: tag, Options: &option.RemoteTLSDNSServerOptions{RemoteDNSServerOptions: remote}}, nil
	case "udp":
		if parsed.Path != "" || net.ParseIP(host) == nil {
			return option.DNSServerOptions{}, invalid
		}
		return option.DNSServerOptions{Type: C.DNSTypeUDP, Tag: tag, Options: &remote}, nil
	}
	return option.DNSServerOptions{}, invalid
}

// Which way names resolve, DNS.Routing.
const (
	// RoutingAuto resolves a name the way its traffic goes: by the rules of the preset, then by its default.
	RoutingAuto = "auto"
	// RoutingProxy resolves every name through the server, RoutingDirect every name past it.
	RoutingProxy  = "proxy"
	RoutingDirect = "direct"
)

// Which address of a site with both a connection takes first, DNS.AddressFirst; "" is as the core decides.
const (
	AddressIPv4 = "ipv4"
	AddressIPv6 = "ipv6"
)

// SystemDNS is the system's DNS as the direct DNS of a preset, which the default of the settings does not replace.
const SystemDNS = "local"

// DNS is how a connection resolves names: the DNS page of the settings. The zero value resolves as the traffic
// goes, with the cache of sing-box.
type DNS struct {
	// Routing is RoutingAuto, RoutingProxy or RoutingDirect; "" is RoutingAuto.
	Routing string
	// FakeIP answers the names programs ask through the TUN with addresses of 198.18.0.0/15, and with IPv6 of
	// 2001:db8::/32 (the TUN keeps fc00::/7 out), and finds the names by them.
	FakeIP bool
	// Hosts answers the names of the hosts file of Windows from it.
	Hosts bool
	// Answers answer their names before the hosts file and every server.
	Answers []Answer
	// NoCache asks the servers every time; the rest of the cache settings need the cache. Optimistic gives an
	// outdated answer at once and asks for a fresh one, so it needs answers that get outdated: not with IgnoreTTL.
	NoCache    bool
	CacheSize  uint32
	IgnoreTTL  bool
	Optimistic bool
	// Timeout is how long a query waits for its server; 0 is sing-box's.
	Timeout time.Duration
	// ClientSubnet tells the servers where queries come from (ECS), so that CDNs answer with near addresses.
	ClientSubnet netip.Prefix
	// AddressFirst is AddressIPv4, AddressIPv6 or "".
	AddressFirst string
	// Object is a dns object of sing-box as JSON, "" for none. It replaces everything above and the DNS of the
	// preset; servers in it reach the server through "detour": "proxy".
	Object string
}

// Answer is a name answered with set addresses.
type Answer struct {
	Name      string
	Addresses []netip.Addr
}

// ParseAnswers reads ready answers: a line each, a name and one or more addresses after it, split by spaces. Empty
// lines and lines starting with # are skipped; a name of a line before it is taken once.
func ParseAnswers(text string) ([]Answer, error) {
	var answers []Answer
	for i, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		name := strings.TrimSuffix(strings.ToLower(fields[0]), ".")
		// A name with an empty label, such as "a..", would never match a query.
		if len(fields) < 2 || name == "" || strings.ContainsAny(name, "/:") || slices.Contains(strings.Split(name, "."), "") {
			return nil, fmt.Errorf("line %d: a name and its addresses", i+1)
		}
		answer := Answer{Name: name}
		for _, field := range fields[1:] {
			addr, err := netip.ParseAddr(field)
			if err != nil {
				return nil, fmt.Errorf("line %d: %q is no address", i+1, field)
			}
			answer.Addresses = append(answer.Addresses, addr.Unmap())
		}
		if slices.ContainsFunc(answers, func(a Answer) bool { return a.Name == name }) {
			return nil, fmt.Errorf("line %d: %s is there already", i+1, name)
		}
		answers = append(answers, answer)
	}
	return answers, nil
}

// CheckDNSServer tells whether text is a DNS server a preset or the settings may name: https://, tls:// or an
// address. Direct ones may be SystemDNS too.
func CheckDNSServer(text string, direct bool) error {
	if direct && text == SystemDNS {
		return nil
	}
	_, err := dnsServer(text, TagDNSRemote, option.DialerOptions{}, TagDNSDirect)
	return err
}

// parseDNSObject reads a dns object of sing-box; unknown keys are refused.
func parseDNSObject(text string) (*option.DNSOptions, error) {
	var options option.DNSOptions
	if err := json.UnmarshalContextDisallowUnknownFields(include.Context(context.Background()), []byte(text), &options); err != nil {
		return nil, fmt.Errorf("sing-box cannot read the dns object: %w", err)
	}
	if err := checkDNSReferences(text); err != nil {
		return nil, err
	}
	return &options, nil
}

// checkDNSReferences refuses what sing-box finds only as it starts: a server that dials through an outbound a
// connection has not, and a rule, a resolver or the final that names a server the object has not.
func checkDNSReferences(text string) error {
	var object struct {
		Servers []map[string]any `json:"servers"`
		Rules   []map[string]any `json:"rules"`
		Final   string           `json:"final"`
	}
	if err := stdjson.Unmarshal([]byte(text), &object); err != nil {
		return fmt.Errorf("the dns object: %w", err)
	}
	tags := map[string]bool{}
	for _, server := range object.Servers {
		if tag, _ := server["tag"].(string); tag != "" {
			tags[tag] = true
		}
	}
	known := func(where, tag string) error {
		if tag != "" && !tags[tag] {
			return fmt.Errorf("%s: unknown DNS server %q", where, tag)
		}
		return nil
	}
	for i, server := range object.Servers {
		where := fmt.Sprintf("servers[%d]", i)
		if detour, _ := server["detour"].(string); detour != "" {
			if _, endpoint := endpointIndex(detour); detour != TagProxy && detour != TagDirect && !endpoint {
				return fmt.Errorf("%s: unknown outbound %q; a server goes through the server with \"detour\": \"proxy\"", where, detour)
			}
		}
		resolver, _ := server["domain_resolver"].(string)
		if options, ok := server["domain_resolver"].(map[string]any); ok {
			resolver, _ = options["server"].(string)
		}
		if err := known(where+".domain_resolver", resolver); err != nil {
			return err
		}
	}
	var walk func(where string, rules []any) error
	walk = func(where string, rules []any) error {
		for i, raw := range rules {
			rule, _ := raw.(map[string]any)
			at := fmt.Sprintf("%s[%d]", where, i)
			if server, _ := rule["server"].(string); server != "" {
				if err := known(at, server); err != nil {
					return err
				}
			}
			if nested, ok := rule["rules"].([]any); ok {
				if err := walk(at+".rules", nested); err != nil {
					return err
				}
			}
		}
		return nil
	}
	rules := make([]any, len(object.Rules))
	for i, rule := range object.Rules {
		rules[i] = rule
	}
	if err := walk("rules", rules); err != nil {
		return err
	}
	return known("final", object.Final)
}

// answerRules answer the names of answers: an A and an AAAA rule each, so that a name with addresses of one family
// gets no address of the other from elsewhere.
func answerRules(answers []Answer) []option.DNSRule {
	var rules []option.DNSRule
	for _, answer := range answers {
		for _, family := range []struct {
			query  uint16
			record string
			is4    bool
		}{{1, "A", true}, {28, "AAAA", false}} {
			var records badoption.Listable[option.DNSRecordOptions]
			for _, addr := range answer.Addresses {
				if addr.Is4() != family.is4 {
					continue
				}
				var record option.DNSRecordOptions
				text := strconv.Quote(fmt.Sprintf("%s. 600 IN %s %s", ace(answer.Name), family.record, addr))
				// The text is a record of a name and an address that were read already.
				_ = record.UnmarshalJSON([]byte(text))
				records = append(records, record)
			}
			rules = append(rules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultDNSRule{
				RawDefaultDNSRule: option.RawDefaultDNSRule{
					Domain:    badoption.Listable[string]{ace(answer.Name)},
					QueryType: badoption.Listable[option.DNSQueryType]{option.DNSQueryType(family.query)},
				},
				DNSRuleAction: option.DNSRuleAction{Action: C.RuleActionTypePredefined, PredefinedOptions: option.DNSRouteActionPredefined{Answer: records}},
			}})
		}
	}
	return rules
}
