package engine

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"PorosCore/config"
	"PorosCore/internal/jsonrpc"
)

// rule is a rule of one condition with one value, as a simple preset has.
func rule(kind, value, action string) PresetRule {
	return PresetRule{Conditions: []RuleCondition{{Kind: kind, Values: []string{value}}}, Action: action}
}

func values(rules []PresetRule) []string {
	var list []string
	for _, rule := range rules {
		var parts []string
		for _, condition := range rule.Conditions {
			parts = append(parts, condition.Kind+":"+strings.Join(condition.Values, ","))
		}
		list = append(list, strings.Join(parts, " "))
	}
	return list
}

func problemOf(t *testing.T, err error) RuleProblem {
	t.Helper()
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("not an error of the core: %v", err)
	}
	problem, ok := rpcErr.Data.(RuleProblem)
	if !ok {
		t.Fatalf("no rule problem: %+v", rpcErr)
	}
	return problem
}

func TestPresetCopies(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	saved, err := e.SavePreset(PresetCopy{
		BasedOn: "all", Name: "Home", DNS: "https://1.1.1.1/dns-query",
		Rules: []PresetRule{
			rule(CondSuffix, "https://Video.Example.com/watch", ActionProxy),
			rule(CondSuffix, "*.ads.example", ActionBlock),
			rule(CondIP, "192.168.50.0/24", ActionDirect),
			rule(CondIP, "203.0.113.7/32", ActionDirect),
			rule(CondProcessName, "steam", ActionDirect),
			rule(CondProcessPath, `C:\Games\game.exe`, ActionProxy),
		},
	})
	if err != nil || saved.ID == "" || saved.Rules != 6 || saved.BasedOn != "all" || saved.Advanced {
		t.Fatalf("SavePreset: %+v, %v", saved, err)
	}
	// The core keeps what a value matches: a link is its site, an address its own subnet, a program its file.
	stored, err := e.PresetCopy(saved.ID)
	want := []string{"suffix:video.example.com", "suffix:ads.example", "ip:192.168.50.0/24", "ip:203.0.113.7", "processName:steam.exe", `processPath:C:\Games\game.exe`}
	if err != nil || !slices.Equal(values(stored.Rules), want) || len(stored.Inherited) != 2 || !stored.Inherited[0].First || stored.Inherited[1].First {
		t.Fatalf("stored: %q, %+v, %v", values(stored.Rules), stored.Inherited, err)
	}

	preset, found, err := e.preset(saved.ID)
	if err != nil || !found || preset.DNS != "https://1.1.1.1/dns-query" || len(preset.Rules) != 8 {
		t.Fatalf("composed: %+v, %v", preset, err)
	}
	// DNS stays first, the user's rules come in their order before the base's own rules.
	first, video, ads, local := preset.Rules[0], preset.Rules[1], preset.Rules[2], preset.Rules[7]
	if first.Action != config.ActionHijackDNS || local.Name != "Local networks" ||
		!slices.Equal(video.DomainSuffix, []string{"video.example.com"}) || video.Outbound != config.OutboundProxy ||
		!slices.Equal(ads.DomainSuffix, []string{"ads.example"}) || ads.Action != config.ActionReject ||
		!slices.Equal(preset.Rules[5].ProcessName, []string{"steam.exe"}) || !slices.Equal(preset.Rules[6].ProcessPath, []string{`C:\Games\game.exe`}) {
		t.Errorf("rules: %+v", preset.Rules)
	}

	settings := DefaultSettings()
	settings.Preset = saved.ID
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatalf("a copy is a preset to choose: %v", err)
	}

	link, err := e.ExportPreset(saved.ID)
	if err != nil || !strings.HasPrefix(link.Link, "poros://route/") {
		t.Fatalf("ExportPreset: %v, %v", link, err)
	}
	imported, err := e.ImportPreset(link)
	if err != nil || imported.ID == saved.ID || imported.Name != "Home 2" || imported.Rules != 6 {
		t.Fatalf("ImportPreset: %+v, %v", imported, err)
	}
	if again, _ := e.PresetCopy(imported.ID); !reflect.DeepEqual(again.Rules, stored.Rules) {
		t.Errorf("the link lost rules: %q", values(again.Rules))
	}
	for _, bad := range []string{"https://example.com", "poros://route/!!!", "poros://route/" + strings.Repeat("A", 8),
		// A link of other fields is no preset of this Poros: a rule could have lost a condition.
		"poros://route/" + encodeLink(`{"kind":"poros-preset","v":1,"name":"Old","sites":[{"value":"example.org","action":"block"}]}`)} {
		if _, err = e.ImportPreset(PresetLink{Link: bad}); reason(err) != ReasonInvalidLink {
			t.Errorf("%q: %v", bad, err)
		}
	}

	// A wrong rule says which one, which value and why.
	broken := stored
	broken.Rules = slices.Clone(stored.Rules)
	broken.Rules[2] = rule(CondIP, "10.8.0.0/33", ActionDirect)
	_, err = e.SavePreset(broken)
	if p := problemOf(t, err); p != (RuleProblem{Reason: ReasonInvalidEntry, Rule: 2, Kind: CondIP, Line: 1, Value: "10.8.0.0/33"}) {
		t.Errorf("a wrong value: %+v", p)
	}
	for name, bad := range map[string]PresetRule{
		ReasonInvalidValue: rule(CondSuffix, "example.com", "elsewhere"),
		ReasonAdvancedOnly: rule(CondPort, "443", ActionDirect),
		ReasonNoCondition:  {Action: ActionProxy},
	} {
		broken.Rules = []PresetRule{bad}
		if _, err = e.SavePreset(broken); reason(err) != name {
			t.Errorf("%s: %v", name, err)
		}
	}
	two := rule(CondSuffix, "example.com", ActionProxy)
	two.Conditions = append(two.Conditions, RuleCondition{Kind: CondIP, Values: []string{"10.0.0.0/8"}})
	broken.Rules = []PresetRule{two}
	if _, err = e.SavePreset(broken); reason(err) != ReasonAdvancedOnly {
		t.Errorf("a simple preset has rules of one condition: %v", err)
	}
	broken.Rules = []PresetRule{rule(CondSuffix, "example.com", ActionProxy), rule(CondSuffix, "Example.com.", ActionBlock)}
	if p := problemOf(t, func() error { _, err := e.SavePreset(broken); return err }()); p.Reason != ReasonDuplicate || p.Rule != 1 || p.Other != 0 {
		t.Errorf("a repeated rule: %+v", p)
	}

	// A connection on a removed copy goes back to the copy's base.
	if err = e.RemovePreset(saved.ID); err != nil {
		t.Fatal(err)
	}
	if settings, _ = e.Settings(); settings.Preset != "all" {
		t.Errorf("after the removal the preset is %q", settings.Preset)
	}
	presets, _ := e.Presets()
	if len(presets) != 3 || presets[2].ID != imported.ID || presets[0].Rules != 2 {
		t.Errorf("presets: %+v", presets)
	}
}

func encodeLink(data string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(data))
}

// The first rule that matches decides, so a narrower rule rises right over the first wider one above it, and the
// rest keep the user's order: a block of a subdomain works under a domain through the server, and so does an
// exception in a subnet.
func TestNarrowerRulesRise(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	draft := PresetCopy{BasedOn: "all", Name: "Order", Rules: []PresetRule{
		rule(CondSuffix, "example.com", ActionProxy),
		rule(CondIP, "10.0.0.0/8", ActionDirect),
		rule(CondSuffix, "*.ads.example.com", ActionBlock),
		rule(CondSuffix, "b.example", ActionProxy),
		rule(CondSuffix, "a.example", ActionDirect),
		rule(CondIP, "10.1.0.0/16", ActionProxy),
		rule(CondIP, "10.1.2.3", ActionBlock),
		rule(CondProcessName, "steam.exe", ActionDirect),
		rule(CondProcessPath, `C:\Games\steam.exe`, ActionProxy),
	}}
	saved, err := e.SavePreset(draft)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"suffix:ads.example.com", "suffix:example.com", "ip:10.1.2.3", "ip:10.1.0.0/16", "ip:10.0.0.0/8",
		"suffix:b.example", "suffix:a.example", `processPath:C:\Games\steam.exe`, "processName:steam.exe"}
	stored, _ := e.PresetCopy(saved.ID)
	if got := values(stored.Rules); !slices.Equal(got, want) {
		t.Errorf("order:\n%q\nwant\n%q", got, want)
	}
	preset, _, _ := e.preset(saved.ID)
	if preset.Rules[1].DomainSuffix[0] != "ads.example.com" || preset.Rules[9].ProcessName[0] != "steam.exe" {
		t.Errorf("the connection checks the rules in the order the editor shows: %+v", preset.Rules)
	}

	// Moving a narrower rule under a wider one does not stick; the kinds of an advanced preset rise as well.
	checked, err := CheckPresetRules(PresetRules{Advanced: true, Rules: []PresetRule{
		rule(CondSuffix, "example.com", ActionProxy),
		rule(CondSuffix, "ads.example.com", ActionBlock),
		rule(CondPort, "8000-9000", ActionDirect),
		rule(CondDomain, "example.com", ActionDirect),
		rule(CondPort, "8080", ActionProxy),
		rule(CondSourceIP, "192.168.0.0/16", ActionDirect),
		rule(CondSourceIP, "192.168.1.7", ActionBlock),
	}})
	want = []string{"suffix:ads.example.com", "domain:example.com", "suffix:example.com", "port:8080", "port:8000-9000", "sourceIP:192.168.1.7", "sourceIP:192.168.0.0/16"}
	if got := values(checked.Rules); err != nil || !slices.Equal(got, want) {
		t.Errorf("advanced order: %q, %v\nwant %q", got, err, want)
	}
}

// Built-in presets never change: the editor shows them, and their IDs are neither saved over nor removed.
func TestBuiltinPresetsStay(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	before, _, _ := e.preset("all")
	shown, err := e.PresetCopy("all")
	if err != nil || !shown.Builtin || shown.ID != "all" || len(shown.Rules) != 0 || shown.Default != ActionProxy ||
		!reflect.DeepEqual(shown.Inherited, []InheritedRule{{Name: "Route DNS", Action: "dns", First: true}, {Name: "Local networks", Action: ActionDirect}}) {
		t.Fatalf("a built-in preset in the editor: %+v, %v", shown, err)
	}
	shown.Rules = []PresetRule{rule(CondSuffix, "example.org", ActionBlock)}
	if _, err = e.SavePreset(shown); reason(err) != ReasonInvalidValue {
		t.Errorf("saving a built-in preset: %v", err)
	}
	shown.Builtin, shown.Name = false, "Changed"
	if _, err = e.SavePreset(shown); reason(err) != ReasonNotFound {
		t.Errorf("saving over a built-in preset: %v", err)
	}
	if err = e.RemovePreset("hardened"); reason(err) != ReasonNotFound {
		t.Errorf("removing a built-in preset: %v", err)
	}
	if _, err = e.ExportPreset("all"); reason(err) != ReasonInvalidValue {
		t.Errorf("a link to a built-in preset: %v", err)
	}
	if after, _, _ := e.preset("all"); !reflect.DeepEqual(before, after) {
		t.Errorf("the built-in preset changed: %+v", after)
	}
	if presets, _ := e.Presets(); len(presets) != 2 {
		t.Errorf("presets: %+v", presets)
	}
}

// A preset made from scratch has no base: it shows none, and runs on the standard rules of all traffic through the
// server, exactly as a copy of "all" with the same rules would.
func TestPresetFromScratch(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	rules := []PresetRule{rule(CondSuffix, "example.org", ActionDirect)}
	saved, err := e.SavePreset(PresetCopy{Name: "Work", Rules: rules})
	if err != nil || saved.BasedOn != "" || saved.Rules != 1 {
		t.Fatalf("SavePreset: %+v, %v", saved, err)
	}
	twin, err := e.SavePreset(PresetCopy{BasedOn: "all", Name: "Work too", Rules: rules})
	if err != nil {
		t.Fatal(err)
	}
	own, _, err := e.preset(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	same, _, _ := e.preset(twin.ID)
	// Two presets cannot share a name; the rest is the same.
	same.Name = own.Name
	if !reflect.DeepEqual(own, same) {
		t.Errorf("from scratch %+v, a copy of all %+v", own, same)
	}
	if edited, err := e.PresetCopy(saved.ID); err != nil || edited.BasedOn != "" || len(edited.Inherited) != 2 || edited.Default != ActionProxy {
		t.Errorf("the editor: %+v, %v", edited, err)
	}

	link, _ := e.ExportPreset(saved.ID)
	if imported, err := e.ImportPreset(link); err != nil || imported.BasedOn != "" {
		t.Errorf("a link keeps no base: %+v, %v", imported, err)
	}
	settings := DefaultSettings()
	settings.Preset = saved.ID
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err = e.RemovePreset(saved.ID); err != nil {
		t.Fatal(err)
	}
	if settings, _ = e.Settings(); settings.Preset != "all" {
		t.Errorf("after the removal the preset is %q, want the standard one", settings.Preset)
	}
}

// The traffic no rule takes goes where the user says, and the choice travels with the preset's link.
func TestPresetDefault(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	draft := PresetCopy{BasedOn: "all", Name: "Direct but", Default: ActionDirect, Rules: []PresetRule{rule(CondSuffix, "example.org", ActionProxy)}}
	saved, err := e.SavePreset(draft)
	if err != nil {
		t.Fatal(err)
	}
	if preset, _, _ := e.preset(saved.ID); preset.DefaultOutbound != config.OutboundDirect {
		t.Errorf("composed default %q", preset.DefaultOutbound)
	}
	if edited, _ := e.PresetCopy(saved.ID); edited.Default != ActionDirect {
		t.Errorf("the editor shows %q", edited.Default)
	}
	link, _ := e.ExportPreset(saved.ID)
	imported, err := e.ImportPreset(link)
	if err != nil {
		t.Fatal(err)
	}
	if preset, _, _ := e.preset(imported.ID); preset.DefaultOutbound != config.OutboundDirect {
		t.Errorf("the link lost the default: %q", preset.DefaultOutbound)
	}
	draft.Default = "elsewhere"
	if _, err = e.SavePreset(draft); reason(err) != ReasonInvalidValue {
		t.Errorf("an unknown default: %v", err)
	}
}

// An advanced preset has rules of several conditions and values, inverted and switched-off ones, every kind, and a
// direct DNS; it never becomes simple again, while a simple one becomes advanced.
func TestAdvancedPreset(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	games := PresetRule{Name: " Games over UDP ", Action: ActionDirect, Conditions: []RuleCondition{
		{Kind: CondProcessName, Values: []string{"game.exe", "launcher", "", "GAME.EXE"}},
		{Kind: CondNetwork, Values: []string{"UDP"}},
	}}
	off := rule(CondPort, "22", ActionDirect)
	off.Off = true
	notMine := rule(CondSuffix, "example.com", ActionProxy)
	notMine.Invert = true
	lan := PresetRule{Action: ActionProxy, Conditions: []RuleCondition{{Kind: CondPrivate, Values: []string{"ignored"}}, {Kind: CondSourceIP, Values: []string{"192.168.1.0/24"}}}}
	draft := PresetCopy{BasedOn: "all", Name: "Home", Advanced: true, DirectDNS: "tls://dns.example", Rules: []PresetRule{
		games, off, notMine, lan,
		rule(CondKeyword, "Tracker", ActionBlock),
		rule(CondRegex, `^ads\.`, ActionBlock),
		rule(CondProtocol, "QUIC", ActionBlock),
		rule(CondIPVersion, "6", ActionBlock),
		rule(CondDomain, "exact.example", ActionDirect),
		rule(CondProcessPathRegex, `(?i)\\games\\`, ActionDirect),
	}}
	saved, err := e.SavePreset(draft)
	if err != nil || !saved.Advanced || saved.Rules != 10 {
		t.Fatalf("SavePreset: %+v, %v", saved, err)
	}
	stored, _ := e.PresetCopy(saved.ID)
	if got := stored.Rules[0]; got.Name != "Games over UDP" || !slices.Equal(got.Conditions[0].Values, []string{"game.exe", "launcher.exe"}) ||
		got.Conditions[1].Values[0] != "udp" || len(stored.Rules[3].Conditions[0].Values) != 0 || stored.Rules[4].Conditions[0].Values[0] != "tracker" {
		t.Errorf("stored: %+v", stored.Rules)
	}
	preset, _, err := e.preset(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The switched-off rule is not in the connection; the others are, between the DNS rule and the local networks.
	if len(preset.Rules) != 11 || preset.DirectDNS != "tls://dns.example" {
		t.Fatalf("composed: %+v", preset)
	}
	if r := preset.Rules[1]; r.Name != "Games over UDP" || !slices.Equal(r.ProcessName, []string{"game.exe", "launcher.exe"}) || !slices.Equal(r.Network, []string{"udp"}) {
		t.Errorf("rule of two conditions: %+v", r)
	}
	if r := preset.Rules[2]; !r.Invert || r.DomainSuffix[0] != "example.com" {
		t.Errorf("inverted rule: %+v", r)
	}
	if r := preset.Rules[3]; !r.IPIsPrivate || r.SourceIPCIDR[0] != "192.168.1.0/24" {
		t.Errorf("private networks from the local network: %+v", r)
	}
	if r := preset.Rules[7]; r.IPVersion != 6 || r.Action != config.ActionReject {
		t.Errorf("IPv6 blocked: %+v", r)
	}

	// The system's DNS as the direct one stays the system's whatever the settings put by default.
	draft.ID, draft.DirectDNS = saved.ID, DNSLocal
	if _, err = e.SavePreset(draft); err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.DNSDirect = "tls://dns.example:853"
	if preset, _, _ = e.preset(saved.ID); withDefaultDNS(preset, settings).DirectDNS != config.SystemDNS {
		t.Errorf("the system's DNS: %q", preset.DirectDNS)
	}
	link, _ := e.ExportPreset(saved.ID)
	if imported, err := e.ImportPreset(link); err != nil || !imported.Advanced || imported.Rules != 10 {
		t.Errorf("a link keeps the kind and the rules: %+v, %v", imported, err)
	}

	simple := draft
	simple.Advanced, simple.Rules, simple.DirectDNS = false, nil, ""
	if _, err = e.SavePreset(simple); reason(err) != ReasonInvalidValue {
		t.Errorf("an advanced preset became simple: %v", err)
	}
	work, err := e.SavePreset(PresetCopy{Name: "Work", Rules: []PresetRule{rule(CondSuffix, "example.org", ActionDirect)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.SavePreset(PresetCopy{ID: work.ID, Name: "Work", DirectDNS: "tls://dns.example"}); reason(err) != ReasonAdvancedOnly {
		t.Errorf("a simple preset with a direct DNS: %v", err)
	}
	if made, err := e.SavePreset(PresetCopy{ID: work.ID, Name: "Work", Advanced: true, Rules: []PresetRule{rule(CondSuffix, "example.org", ActionDirect)}}); err != nil || !made.Advanced {
		t.Errorf("a simple preset made advanced: %+v, %v", made, err)
	}

	for want, bad := range map[RuleProblem]PresetRule{
		{Reason: ReasonInvalidEntry, Kind: CondIPVersion, Line: 2, Value: "6"}:  {Action: ActionBlock, Conditions: []RuleCondition{{Kind: CondIPVersion, Values: []string{"4", "6"}}}},
		{Reason: ReasonInvalidEntry, Kind: CondPort, Line: 2, Value: "9000-80"}: {Action: ActionBlock, Conditions: []RuleCondition{{Kind: CondPort, Values: []string{"80", "9000-80"}}}},
		{Reason: ReasonInvalidEntry, Kind: CondRegex, Line: 1, Value: "(ads"}:   rule(CondRegex, "(ads", ActionBlock),
		{Reason: ReasonInvalidEntry, Kind: CondProtocol, Line: 1, Value: "ftp"}: rule(CondProtocol, "ftp", ActionBlock),
		{Reason: ReasonEmptyCondition, Kind: CondNetwork}:                       {Action: ActionBlock, Conditions: []RuleCondition{{Kind: CondNetwork, Values: []string{" "}}}},
		{Reason: ReasonInvalidValue, Kind: CondPort}: {Action: ActionBlock, Conditions: []RuleCondition{
			{Kind: CondPort, Values: []string{"80"}}, {Kind: CondPort, Values: []string{"443"}},
		}},
		{Reason: ReasonInvalidValue, Kind: "geosite"}: rule("geosite", "ads", ActionBlock),
	} {
		_, err := CheckPresetRules(PresetRules{Advanced: true, Rules: []PresetRule{bad}})
		if got := problemOf(t, err); got != want {
			t.Errorf("%+v: got %+v", bad, got)
		}
	}
}

// presets.check answers through JSON-RPC with the checked rules, or with the problem as the data of its error.
func TestPresetCheckThroughRPC(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	check := e.Handlers()[MethodPresetsCheck]
	result, err := check(t.Context(), json.RawMessage(`{"advanced":false,"rules":[`+
		`{"name":"","conditions":[{"kind":"suffix","values":["example.com"]}],"invert":false,"off":false,"action":"proxy"},`+
		`{"name":"","conditions":[{"kind":"suffix","values":["ads.example.com"]}],"invert":false,"off":false,"action":"block"}]}`))
	if got, ok := result.(PresetRules); err != nil || !ok || !slices.Equal(values(got.Rules), []string{"suffix:ads.example.com", "suffix:example.com"}) {
		t.Errorf("checked: %+v, %v", result, err)
	}
	_, err = check(t.Context(), json.RawMessage(`{"advanced":false,"rules":[{"name":"","conditions":[{"kind":"ip","values":["not an address"]}],"invert":false,"off":false,"action":"direct"}]}`))
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("a wrong rule: %v", err)
	}
	data, _ := json.Marshal(rpcErr)
	if !strings.Contains(string(data), `"data":{"reason":"invalid_entry","rule":0,"kind":"ip","line":1,"value":"not an address","other":0}`) {
		t.Errorf("the error says which rule and value: %s", data)
	}
}

// vpnProfiles adds an OpenVPN, an OpenConnect and a Trojan profile to a group of the user's and gives their IDs.
func vpnProfiles(t *testing.T, e *Engine) (ovpn, oc, trojan string) {
	t.Helper()
	certificate, _ := testCertificate(t)
	added, err := e.AddServers(AddServers{NewGroup: "Office", Text: strings.Join([]string{
		porosLink(t, `{"type":"openvpn","name":"Office","server":"vpn.example.com","server_port":1194,`+
			`"openvpn":{"network":"udp","username":"u","password":"pw","ca":`+jsonText(t, certificate)+`}}`),
		porosLink(t, `{"type":"openconnect","name":"Lab","server":"gw.example.com","server_port":443,"openconnect":{"username":"u","password":"pw"}}`),
		"trojan://secret@de.example.com:443#Germany",
	}, "\n")})
	if err != nil || len(added.IDs) != 3 {
		t.Fatalf("AddServers: %+v, %v", added, err)
	}
	return added.IDs[0], added.IDs[1], added.IDs[2]
}

// Endpoints of an advanced preset are OpenVPN and OpenConnect profiles of this computer: the connection reaches them
// by their place, a link leaves them out with the rules to them, and the lists window has no line for those rules.
func TestPresetEndpoints(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	ovpn, oc, trojan := vpnProfiles(t, e)
	wiki := rule(CondSuffix, "wiki.example", ActionEndpoint)
	wiki.Endpoint = oc
	site := rule(CondSuffix, "example.org", ActionDirect)
	site.Endpoint = oc
	draft := PresetCopy{BasedOn: "all", Name: "Office", Advanced: true, Endpoints: []string{ovpn, oc}, Rules: []PresetRule{wiki, site}}
	saved, err := e.SavePreset(draft)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := e.PresetCopy(saved.ID)
	if err != nil || !slices.Equal(stored.Endpoints, []string{ovpn, oc}) || stored.Rules[0].Endpoint != oc || stored.Rules[1].Endpoint != "" {
		t.Fatalf("the endpoints and the rule to one stay; another action has no endpoint: %+v, %v", stored, err)
	}
	preset, _, err := e.preset(saved.ID)
	if err != nil || !slices.Equal(preset.Endpoints, []string{ovpn, oc}) || preset.Rules[1].Outbound != config.EndpointTag(1) {
		t.Fatalf("the connection reaches an endpoint by its place: %+v, %v", preset, err)
	}

	for name, c := range map[string]struct {
		endpoints []string
		advanced  bool
		reason    string
	}{
		"a simple preset":        {[]string{ovpn}, false, ReasonAdvancedOnly},
		"twice":                  {[]string{ovpn, ovpn}, true, ReasonInvalidValue},
		"no VPN":                 {[]string{trojan}, true, ReasonInvalidValue},
		"a profile that is gone": {[]string{"gone"}, true, ReasonNotFound},
	} {
		wrong := PresetCopy{BasedOn: "all", Name: "Wrong", Advanced: c.advanced, Endpoints: c.endpoints, Rules: []PresetRule{}}
		if _, err := e.SavePreset(wrong); reason(err) != c.reason {
			t.Errorf("%s: %v, want %s", name, err, c.reason)
		}
	}
	stray := draft
	stray.Endpoints = []string{ovpn}
	if _, err := e.SavePreset(stray); problemOf(t, err) != (RuleProblem{Reason: ReasonInvalidValue, Rule: 0}) {
		t.Errorf("a rule to an endpoint the preset has not: %v", err)
	}
	if _, err := CheckPresetRules(PresetRules{Rules: []PresetRule{wiki}, Endpoints: []string{oc}}); problemOf(t, err).Reason != ReasonAdvancedOnly {
		t.Errorf("a simple preset has no rules to endpoints: %v", err)
	}

	link, err := e.ExportPreset(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := e.ImportPreset(link)
	if err != nil {
		t.Fatal(err)
	}
	if copy, _ := e.PresetCopy(imported.ID); len(copy.Endpoints) != 0 || !slices.Equal(values(copy.Rules), []string{"suffix:example.org"}) {
		t.Errorf("a link leaves out the endpoints and the rules to them: %+v", copy)
	}

	lists, err := WritePresetLists(PresetRules{Advanced: true, Rules: stored.Rules})
	if err != nil || lists.Lists[ActionProxy] != "" || lists.Lists[ActionDirect] != "suffix:example.org" {
		t.Errorf("a rule to an endpoint has no line: %+v, %v", lists, err)
	}
	read, err := ReadPresetLists(PresetListsInput{Advanced: true, Rules: stored.Rules, Lists: map[string]string{
		ActionProxy: "suffix:Wiki.example", ActionDirect: "suffix:example.org",
	}})
	if err != nil || !reflect.DeepEqual(read.Rules, stored.Rules) ||
		!reflect.DeepEqual(read.Problems, []PresetListProblem{{Action: ActionProxy, Line: 1, Text: "suffix:Wiki.example", Reason: ReasonDuplicate, Other: ActionEndpoint}}) {
		t.Errorf("a line that repeats a rule to an endpoint: %+v, %v", read, err)
	}
}
