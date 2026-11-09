package engine

import (
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"PorosCore/config"
	"PorosCore/internal/jsonrpc"
)

// simpleKinds are the kinds of a simple preset's rules; an advanced preset takes ruleKinds, all of them.
var (
	simpleKinds = []string{CondSuffix, CondIP, CondProcessName, CondProcessPath}
	ruleKinds   = []string{
		CondSuffix, CondDomain, CondKeyword, CondRegex, CondIP, CondPrivate, CondPort, CondProcessName, CondProcessPath,
		CondProcessPathRegex, CondProtocol, CondNetwork, CondIPVersion, CondSourceIP,
	}
	ruleActions = []string{ActionProxy, ActionDirect, ActionBlock}
	// protocols are those sing-box sniffs.
	protocols     = []string{"tls", "http", "quic", "dns", "stun", "bittorrent", "dtls", "ssh", "rdp", "ntp"}
	fileExtension = regexp.MustCompile(`\.[A-Za-z0-9]+$`)
)

func ruleFailure(problem RuleProblem, format string, args ...any) error {
	return &jsonrpc.Error{Code: jsonrpc.CodeFailed, Message: fmt.Sprintf(format, args...), Data: problem}
}

// CheckPresetRules gives the rules of a preset as the core keeps them: every value read for its kind, repeats in a
// condition dropped, and a narrower rule over a wider one. A rule that is wrong fails with a RuleProblem.
func CheckPresetRules(p PresetRules) (PresetRules, error) {
	rules, err := checkRules(p.Advanced, p.Endpoints, p.Rules)
	return PresetRules{Advanced: p.Advanced, Rules: rules, Endpoints: p.Endpoints}, err
}

// checkRules checks the rules of a preset whose endpoints are the profiles endpoints names.
func checkRules(advanced bool, endpoints []string, rules []PresetRule) ([]PresetRule, error) {
	checked := make([]PresetRule, 0, len(rules))
	seen := map[string]int{}
	for i, rule := range rules {
		rule, err := checkRule(advanced, endpoints, i, rule)
		if err != nil {
			return nil, err
		}
		if key, ok := singleKey(rule); ok {
			if other, repeated := seen[key]; repeated {
				return nil, ruleFailure(RuleProblem{Reason: ReasonDuplicate, Rule: i, Kind: rule.Conditions[0].Kind, Line: 1, Value: rule.Conditions[0].Values[0], Other: other},
					"rule %d repeats rule %d", i+1, other+1)
			}
			seen[key] = i
		}
		checked = append(checked, rule)
	}
	return narrowFirst(checked), nil
}

func checkRule(advanced bool, endpoints []string, i int, rule PresetRule) (PresetRule, error) {
	problem := func(reason string) RuleProblem { return RuleProblem{Reason: reason, Rule: i} }
	switch {
	case rule.Action == ActionEndpoint && !advanced:
		return rule, ruleFailure(problem(ReasonAdvancedOnly), "rule %d: only an advanced preset has endpoints", i+1)
	case rule.Action == ActionEndpoint && !slices.Contains(endpoints, rule.Endpoint):
		return rule, ruleFailure(problem(ReasonInvalidValue), "rule %d: the preset has no endpoint %q", i+1, rule.Endpoint)
	case rule.Action != ActionEndpoint && !slices.Contains(ruleActions, rule.Action):
		return rule, ruleFailure(problem(ReasonInvalidValue), "rule %d: the action must be proxy, direct, block or endpoint, not %q", i+1, rule.Action)
	case rule.Action != ActionEndpoint:
		rule.Endpoint = ""
	}
	if len(rule.Conditions) == 0 {
		return rule, ruleFailure(problem(ReasonNoCondition), "rule %d has no condition", i+1)
	}
	rule.Name = strings.TrimSpace(rule.Name)
	if !advanced && (len(rule.Conditions) > 1 || len(rule.Conditions[0].Values) > 1 || rule.Invert || rule.Off || rule.Name != "") {
		return rule, ruleFailure(problem(ReasonAdvancedOnly), "rule %d: a simple preset has rules of one condition with one value", i+1)
	}
	conditions := make([]RuleCondition, 0, len(rule.Conditions))
	for _, condition := range rule.Conditions {
		p := problem("")
		p.Kind = condition.Kind
		switch {
		case !slices.Contains(ruleKinds, condition.Kind):
			p.Reason = ReasonInvalidValue
			return rule, ruleFailure(p, "rule %d: there is no kind %q", i+1, condition.Kind)
		case !advanced && !slices.Contains(simpleKinds, condition.Kind):
			p.Reason = ReasonAdvancedOnly
			return rule, ruleFailure(p, "rule %d: %s belongs to an advanced preset", i+1, condition.Kind)
		case slices.ContainsFunc(conditions, func(c RuleCondition) bool { return c.Kind == condition.Kind }):
			p.Reason = ReasonInvalidValue
			return rule, ruleFailure(p, "rule %d has two conditions of %s", i+1, condition.Kind)
		}
		values := []string{}
		for line, text := range condition.Values {
			if condition.Kind == CondPrivate {
				break
			}
			if strings.TrimSpace(text) == "" {
				continue
			}
			value, ok := ruleValue(condition.Kind, text)
			if !ok {
				p.Reason, p.Line, p.Value = ReasonInvalidEntry, line+1, strings.TrimSpace(text)
				return rule, ruleFailure(p, "rule %d: %q is no %s", i+1, p.Value, condition.Kind)
			}
			if !slices.ContainsFunc(values, func(v string) bool { return valueKey(condition.Kind, v) == valueKey(condition.Kind, value) }) {
				values = append(values, value)
			}
		}
		if condition.Kind != CondPrivate && len(values) == 0 {
			p.Reason = ReasonEmptyCondition
			return rule, ruleFailure(p, "rule %d: the condition of %s has no values", i+1, condition.Kind)
		}
		if condition.Kind == CondIPVersion && len(values) > 1 {
			// Both versions are no condition at all.
			p.Reason, p.Line, p.Value = ReasonInvalidEntry, 2, values[1]
			return rule, ruleFailure(p, "rule %d: an IP version is 4 or 6", i+1)
		}
		conditions = append(conditions, RuleCondition{Kind: condition.Kind, Values: values})
	}
	rule.Conditions = conditions
	return rule, nil
}

// ruleValue reads a value of a condition of kind as the core keeps it.
func ruleValue(kind, text string) (string, bool) {
	value := strings.TrimSpace(text)
	switch kind {
	case CondSuffix, CondDomain:
		if strings.Contains(value, "://") {
			if parsed, err := url.Parse(value); err == nil {
				value = parsed.Hostname()
			}
		}
		value = strings.TrimSuffix(strings.ToLower(value), ".")
		if kind == CondSuffix {
			value = strings.TrimPrefix(strings.TrimPrefix(value, "*."), ".")
		}
		return value, isDomain(value)
	case CondKeyword:
		value = strings.ToLower(value)
		return value, value != "" && !strings.ContainsFunc(value, isSpace)
	case CondRegex, CondProcessPathRegex:
		_, err := regexp.Compile(value)
		return value, value != "" && err == nil
	case CondIP, CondSourceIP:
		if prefix, err := netip.ParsePrefix(value); err == nil {
			if prefix.Masked().Bits() == prefix.Addr().BitLen() {
				return prefix.Addr().Unmap().String(), true
			}
			return prefix.Masked().String(), true
		}
		if addr, err := netip.ParseAddr(strings.Trim(value, "[]")); err == nil {
			return addr.Unmap().String(), true
		}
		return value, false
	case CondPort:
		first, last, ok := readPorts(value)
		if !ok {
			return value, false
		}
		if first == last {
			return strconv.Itoa(int(first)), true
		}
		return fmt.Sprintf("%d-%d", first, last), true
	case CondProcessName:
		if value == "" || strings.ContainsAny(value, `\/*?"<>|:`) {
			return value, false
		}
		// Windows names the process by its file, extension included.
		if runtime.GOOS == "windows" && !fileExtension.MatchString(value) {
			value += ".exe"
		}
		return value, true
	case CondProcessPath:
		return value, strings.ContainsAny(value, `\/`) && !strings.ContainsAny(value, `*?"<>|`)
	case CondProtocol:
		value = strings.ToLower(value)
		return value, slices.Contains(protocols, value)
	case CondNetwork:
		value = strings.ToLower(value)
		return value, value == "tcp" || value == "udp"
	case CondIPVersion:
		return value, value == "4" || value == "6"
	}
	return value, false
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }

// readPorts reads a port, or a range written first-last or first:last.
func readPorts(text string) (first, last uint16, ok bool) {
	a, b, isRange := strings.Cut(strings.ReplaceAll(text, ":", "-"), "-")
	from, err := strconv.ParseUint(strings.TrimSpace(a), 10, 16)
	if err != nil || from == 0 {
		return 0, 0, false
	}
	if !isRange {
		return uint16(from), uint16(from), true
	}
	to, err := strconv.ParseUint(strings.TrimSpace(b), 10, 16)
	if err != nil || to < from {
		return 0, 0, false
	}
	return uint16(from), uint16(to), true
}

// valueKey is what a value matches: programs are named whatever their case, as Windows does.
func valueKey(kind, value string) string {
	if kind == CondProcessName || kind == CondProcessPath {
		return kind + " " + strings.ToLower(value)
	}
	return kind + " " + value
}

// single tells a rule of one condition with one value that is not inverted: the rule of a simple preset, and of a
// line of the lists window.
func single(rule PresetRule) bool {
	return len(rule.Conditions) == 1 && len(rule.Conditions[0].Values) == 1 && !rule.Invert
}

func singleKey(rule PresetRule) (string, bool) {
	if !single(rule) {
		return "", false
	}
	return valueKey(rule.Conditions[0].Kind, rule.Conditions[0].Values[0]), true
}

// narrower tells a single rule that matches only part of what another one does: a subdomain or the exact domain
// under a domain with its subdomains, a smaller subnet in a larger one, a port in a range, a program's path under
// its file name.
func narrower(a, b PresetRule) bool {
	if !single(a) || !single(b) {
		return false
	}
	ka, va, kb, vb := a.Conditions[0].Kind, a.Conditions[0].Values[0], b.Conditions[0].Kind, b.Conditions[0].Values[0]
	switch {
	case kb == CondSuffix && (ka == CondSuffix || ka == CondDomain):
		return strings.HasSuffix(va, "."+vb) || (ka == CondDomain && va == vb)
	case ka == kb && (ka == CondIP || ka == CondSourceIP):
		pa, errA := netip.ParsePrefix(prefixText(va))
		pb, errB := netip.ParsePrefix(prefixText(vb))
		return errA == nil && errB == nil && pa.Addr().Is4() == pb.Addr().Is4() && pa.Bits() > pb.Bits() && pb.Contains(pa.Addr())
	case ka == CondPort && kb == CondPort:
		fa, la, okA := readPorts(va)
		fb, lb, okB := readPorts(vb)
		return okA && okB && fb <= fa && la <= lb && la-fa < lb-fb
	case ka == CondProcessPath && kb == CondProcessName:
		return strings.EqualFold(va[strings.LastIndexAny(va, `\/`)+1:], vb)
	}
	return false
}

// prefixText writes an address as the subnet of itself alone.
func prefixText(value string) string {
	if strings.Contains(value, "/") {
		return value
	}
	if addr, err := netip.ParseAddr(value); err == nil {
		return netip.PrefixFrom(addr, addr.BitLen()).String()
	}
	return value
}

// narrowFirst keeps the user's order but lifts each narrower rule right over the first wider one above it, so that
// the narrower one decides wherever both match: a block of ads.example.com works under example.com through the
// server.
func narrowFirst(rules []PresetRule) []PresetRule {
	rules = slices.Clone(rules)
	for i := range rules {
		for j := 0; j < i; j++ {
			if narrower(rules[i], rules[j]) {
				rule := rules[i]
				copy(rules[j+1:i+1], rules[j:i])
				rules[j] = rule
				break
			}
		}
	}
	return rules
}

// configRule is a checked rule of a preset as the config builder takes it.
// configRule turns a checked rule into one of the connection; endpoints are those of its preset.
func configRule(rule PresetRule, endpoints []string) config.Rule {
	out := config.Rule{Name: rule.Name, Invert: rule.Invert}
	if out.Name == "" && len(rule.Conditions) > 0 && len(rule.Conditions[0].Values) > 0 {
		out.Name = rule.Conditions[0].Values[0]
	}
	switch rule.Action {
	case ActionBlock:
		out.Action = config.ActionReject
	case ActionEndpoint:
		out.Action, out.Outbound = config.ActionRoute, config.EndpointTag(slices.Index(endpoints, rule.Endpoint))
	default:
		out.Action, out.Outbound = config.ActionRoute, rule.Action
	}
	for _, condition := range rule.Conditions {
		values := condition.Values
		switch condition.Kind {
		case CondSuffix:
			out.DomainSuffix = values
		case CondDomain:
			out.Domain = values
		case CondKeyword:
			out.DomainKeyword = values
		case CondRegex:
			out.DomainRegex = values
		case CondIP:
			out.IPCIDR = values
		case CondPrivate:
			out.IPIsPrivate = true
		case CondSourceIP:
			out.SourceIPCIDR = values
		case CondPort:
			for _, value := range values {
				first, last, _ := readPorts(value)
				if first == last {
					out.Port = append(out.Port, first)
				} else {
					out.PortRange = append(out.PortRange, fmt.Sprintf("%d:%d", first, last))
				}
			}
		case CondProcessName:
			out.ProcessName = values
		case CondProcessPath:
			out.ProcessPath = values
		case CondProcessPathRegex:
			out.ProcessPathRegex = values
		case CondProtocol:
			out.Protocol = values
		case CondNetwork:
			out.Network = values
		case CondIPVersion:
			out.IPVersion, _ = strconv.Atoi(values[0])
		}
	}
	return out
}

func isDomain(value string) bool {
	if value == "" || len(value) > 253 || !strings.Contains(value, ".") {
		return false
	}
	for label := range strings.SplitSeq(value, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 0x7f) {
				return false
			}
		}
	}
	return true
}
