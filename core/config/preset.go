package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
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
	Rules           []Rule `json:"rules"`
}

// Rule matches a connection by any of its conditions, the way a sing-box rule does: a destination matches one
// address condition (domain, suffix, keyword, regex, CIDR, private) and, when set, one of the other kinds as well.
type Rule struct {
	Name string `json:"name,omitempty"`
	// Action is route, reject or hijack-dns; Outbound is proxy or direct and goes with route.
	Action       string   `json:"action"`
	Outbound     string   `json:"outbound,omitempty"`
	Protocol     []string `json:"protocol,omitempty"`
	Domain       []string `json:"domain,omitempty"`
	DomainSuffix []string `json:"domain_suffix,omitempty"`
	// DomainKeyword and DomainRegex come from imported presets; the simple mode writes domains and suffixes.
	DomainKeyword []string `json:"domain_keyword,omitempty"`
	DomainRegex   []string `json:"domain_regex,omitempty"`
	IPCIDR        []string `json:"ip_cidr,omitempty"`
	IPIsPrivate   bool     `json:"ip_is_private,omitempty"`
	ProcessName   []string `json:"process_name,omitempty"`
	ProcessPath   []string `json:"process_path,omitempty"`
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
	}
	return nil
}

func (r Rule) validate() error {
	switch r.Action {
	case ActionRoute:
		if r.Outbound != OutboundProxy && r.Outbound != OutboundDirect {
			return fmt.Errorf("unknown outbound %q", r.Outbound)
		}
	case ActionReject, ActionHijackDNS:
		if r.Outbound != "" {
			return fmt.Errorf("%s takes no outbound", r.Action)
		}
	default:
		return fmt.Errorf("unknown action %q", r.Action)
	}
	if len(r.Protocol)+len(r.Domain)+len(r.DomainSuffix)+len(r.DomainKeyword)+len(r.DomainRegex)+len(r.IPCIDR)+
		len(r.ProcessName)+len(r.ProcessPath) == 0 && !r.IPIsPrivate {
		// A rule without conditions would take everything after it.
		return errors.New("the rule has no condition")
	}
	for _, list := range [][]string{r.Protocol, r.Domain, r.DomainSuffix, r.DomainKeyword, r.DomainRegex, r.ProcessName, r.ProcessPath} {
		if slices.ContainsFunc(list, func(item string) bool { return strings.TrimSpace(item) == "" }) {
			return errors.New("the rule has an empty value")
		}
	}
	for _, cidr := range r.IPCIDR {
		if _, err := parsePrefix(cidr); err != nil {
			return err
		}
	}
	return nil
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
