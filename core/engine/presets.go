package engine

import (
	"cmp"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"strings"

	"github.com/sagernet/bbolt"

	"PorosCore/config"
	"PorosCore/internal/jsonrpc"
)

//go:embed presets/*.json
var presetFiles embed.FS

// The order of the mockups: everything, then the hardened one.
var builtinPresetIDs = []string{"all", "hardened"}

type presetRecord struct {
	ID      string
	Builtin bool
	Preset  config.Preset
}

func builtinPresets() ([]presetRecord, error) {
	presets := make([]presetRecord, 0, len(builtinPresetIDs))
	for _, id := range builtinPresetIDs {
		data, err := presetFiles.ReadFile("presets/" + id + ".json")
		if err != nil {
			return nil, err
		}
		preset, err := config.ParsePreset(data)
		if err != nil {
			return nil, fmt.Errorf("preset %s: %w", id, err)
		}
		presets = append(presets, presetRecord{ID: id, Builtin: true, Preset: preset})
	}
	return presets, nil
}

// standardPreset gives the rules of a preset made from scratch, which has no base: all traffic through the server
// and local networks direct.
const standardPreset = "all"

// copyRecord is a user's preset as stored: the entries the user added, in order. BasedOn is the built-in preset it
// is a copy of, "" for one made from scratch.
type copyRecord struct {
	ID      string        `json:"id"`
	BasedOn string        `json:"basedOn"`
	Name    string        `json:"name"`
	Sites   []PresetEntry `json:"sites"`
	Apps    []PresetEntry `json:"apps"`
	DNS     string        `json:"dns"`
	AddedAt int64         `json:"addedAt"`
}

func (r copyRecord) view() Preset {
	return Preset{ID: r.ID, Name: r.Name, BasedOn: r.BasedOn, Sites: len(r.Sites), Apps: len(r.Apps)}
}

// foundation is the built-in preset whose rules a user's preset adds to.
func (e *Engine) foundation(basedOn string) (presetRecord, bool) {
	if basedOn == "" {
		basedOn = standardPreset
	}
	return e.builtin(basedOn)
}

func (e *Engine) builtin(id string) (presetRecord, bool) {
	for _, record := range e.presets {
		if record.ID == id {
			return record, true
		}
	}
	return presetRecord{}, false
}

func (e *Engine) copies() (list []copyRecord, err error) {
	err = e.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketPresets).ForEach(func(key, _ []byte) error {
			var record copyRecord
			if _, err := getJSON(tx, bucketPresets, key, &record); err != nil {
				return err
			}
			list = append(list, record)
			return nil
		})
	})
	slices.SortStableFunc(list, func(a, b copyRecord) int { return cmp.Compare(a.AddedAt, b.AddedAt) })
	return list, err
}

// Presets lists the built-in presets, then the user's copies in the order they were made.
func (e *Engine) Presets() ([]Preset, error) {
	copies, err := e.copies()
	if err != nil {
		return nil, err
	}
	views := make([]Preset, 0, len(e.presets)+len(copies))
	for _, record := range e.presets {
		views = append(views, Preset{ID: record.ID, Name: record.Preset.Name, Builtin: true})
	}
	for _, record := range copies {
		views = append(views, record.view())
	}
	return views, nil
}

// preset gives the preset the connection runs with: a built-in one, or a copy put together with its base.
func (e *Engine) preset(id string) (config.Preset, bool, error) {
	if record, ok := e.builtin(id); ok {
		return record.Preset, true, nil
	}
	var record copyRecord
	var found bool
	err := e.db.View(func(tx *bbolt.Tx) error {
		var err error
		found, err = getJSON(tx, bucketPresets, []byte(id), &record)
		return err
	})
	if err != nil || !found {
		return config.Preset{}, false, err
	}
	preset, err := e.compose(record)
	return preset, err == nil, err
}

// compose puts a copy together: the DNS rules of its base first, then the user's entries, narrower ones first, then
// the rest of the base. The first rule that matches decides, so the user's entries win over the base.
func (e *Engine) compose(record copyRecord) (config.Preset, error) {
	base, ok := e.foundation(record.BasedOn)
	if !ok {
		return config.Preset{}, fmt.Errorf("the copy is based on %q, which is no built-in preset", record.BasedOn)
	}
	preset := base.Preset
	preset.Name, preset.DNS = record.Name, record.DNS
	head := 0
	for head < len(base.Preset.Rules) && base.Preset.Rules[head].Action == config.ActionHijackDNS {
		head++
	}
	var own []config.Rule
	for _, entry := range record.Sites {
		rule, err := siteRule(entry)
		if err != nil {
			return config.Preset{}, err
		}
		own = append(own, rule)
	}
	for _, entry := range record.Apps {
		rule, err := appRule(entry)
		if err != nil {
			return config.Preset{}, err
		}
		own = append(own, rule)
	}
	slices.SortStableFunc(own, func(a, b config.Rule) int {
		groupA, widthA := ruleWidth(a)
		groupB, widthB := ruleWidth(b)
		return cmp.Or(cmp.Compare(groupA, groupB), cmp.Compare(widthA, widthB))
	})
	preset.Rules = slices.Concat(base.Preset.Rules[:head], own, base.Preset.Rules[head:])
	return preset, preset.Validate()
}

// ruleWidth orders the user's entries so that the narrower one decides wherever two match: a subdomain before its
// domain, an address or a smaller subnet before a larger one, a program's path before its name. Domains, addresses
// and programs keep their groups, and entries alike keep the user's order.
func ruleWidth(rule config.Rule) (group, width int) {
	switch {
	case len(rule.DomainSuffix) > 0:
		return 0, -strings.Count(rule.DomainSuffix[0], ".")
	case len(rule.IPCIDR) > 0:
		if prefix, err := netip.ParsePrefix(rule.IPCIDR[0]); err == nil {
			return 1, -prefix.Bits()
		}
		if addr, err := netip.ParseAddr(rule.IPCIDR[0]); err == nil {
			return 1, -addr.BitLen()
		}
		return 1, 0
	case len(rule.ProcessPath) > 0:
		return 2, 0
	default:
		return 3, 0
	}
}

func entryAction(entry PresetEntry, rule *config.Rule) error {
	switch entry.Action {
	case ActionProxy, ActionDirect:
		rule.Action, rule.Outbound = config.ActionRoute, entry.Action
	case ActionBlock:
		rule.Action = config.ActionReject
	default:
		return jsonrpc.Failed(ReasonInvalidEntry, "%q: the action must be proxy, direct or block, not %q", entry.Value, entry.Action)
	}
	return nil
}

// siteRule reads what a user types for a site: a domain with its subdomains, a link to it, an address or a subnet.
func siteRule(entry PresetEntry) (config.Rule, error) {
	rule := config.Rule{Name: entry.Value}
	if err := entryAction(entry, &rule); err != nil {
		return rule, err
	}
	value := strings.TrimSpace(entry.Value)
	if strings.Contains(value, "://") {
		if parsed, err := url.Parse(value); err == nil {
			value = parsed.Hostname()
		}
	}
	value = strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(value), "*."), ".")
	if prefix, err := netip.ParsePrefix(value); err == nil {
		rule.IPCIDR = []string{prefix.Masked().String()}
		return rule, nil
	}
	if addr, err := netip.ParseAddr(strings.Trim(value, "[]")); err == nil {
		rule.IPCIDR = []string{addr.Unmap().String()}
		return rule, nil
	}
	if !isDomain(value) {
		return rule, jsonrpc.Failed(ReasonInvalidEntry, "%q is no site, address or subnet", entry.Value)
	}
	rule.DomainSuffix = []string{value}
	return rule, nil
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

// appRule reads a program: a file name such as steam.exe, or a full path.
func appRule(entry PresetEntry) (config.Rule, error) {
	rule := config.Rule{Name: entry.Value}
	if err := entryAction(entry, &rule); err != nil {
		return rule, err
	}
	value := strings.TrimSpace(entry.Value)
	switch {
	case value == "" || strings.ContainsAny(value, `*?"<>|`):
		return rule, jsonrpc.Failed(ReasonInvalidEntry, "%q is no program", entry.Value)
	case strings.ContainsAny(value, `\/`):
		rule.ProcessPath = []string{value}
	default:
		rule.ProcessName = []string{value}
	}
	return rule, nil
}

func inherited(base config.Preset) []InheritedRule {
	rules := []InheritedRule{}
	for _, rule := range base.Rules {
		switch {
		case rule.Action == config.ActionHijackDNS:
		case rule.Action == config.ActionReject:
			rules = append(rules, InheritedRule{Name: rule.Name, Action: ActionBlock})
		default:
			rules = append(rules, InheritedRule{Name: rule.Name, Action: rule.Outbound})
		}
	}
	return rules
}

// PresetCopy gives a user's preset for the editor; a built-in ID gives a new copy of that preset, and "" a new
// preset from scratch.
func (e *Engine) PresetCopy(id string) (PresetCopy, error) {
	if id == "" {
		base, _ := e.foundation("")
		return PresetCopy{Sites: []PresetEntry{}, Apps: []PresetEntry{}, Inherited: inherited(base.Preset)}, nil
	}
	if base, ok := e.builtin(id); ok {
		return PresetCopy{BasedOn: id, Name: base.Preset.Name, Sites: []PresetEntry{}, Apps: []PresetEntry{}, Inherited: inherited(base.Preset)}, nil
	}
	var record copyRecord
	var found bool
	if err := e.db.View(func(tx *bbolt.Tx) error {
		var err error
		found, err = getJSON(tx, bucketPresets, []byte(id), &record)
		return err
	}); err != nil {
		return PresetCopy{}, err
	}
	if !found {
		return PresetCopy{}, jsonrpc.Failed(ReasonNotFound, "there is no preset %q", id)
	}
	base, ok := e.foundation(record.BasedOn)
	if !ok {
		return PresetCopy{}, jsonrpc.Failed(ReasonUnknownPreset, "the copy is based on %q, which is no built-in preset", record.BasedOn)
	}
	return PresetCopy{
		ID: record.ID, BasedOn: record.BasedOn, Name: record.Name,
		Sites: nonNil(record.Sites), Apps: nonNil(record.Apps), DNS: record.DNS, Inherited: inherited(base.Preset),
	}, nil
}

func nonNil(entries []PresetEntry) []PresetEntry {
	if entries == nil {
		return []PresetEntry{}
	}
	return entries
}

// SavePreset stores a user's preset, a new one when it has no ID.
func (e *Engine) SavePreset(c PresetCopy) (Preset, error) {
	record := copyRecord{
		ID: c.ID, BasedOn: c.BasedOn, Name: strings.TrimSpace(c.Name),
		Sites: nonNil(c.Sites), Apps: nonNil(c.Apps), DNS: strings.TrimSpace(c.DNS),
	}
	if record.Name == "" {
		return Preset{}, jsonrpc.Failed(ReasonInvalidValue, "the preset needs a name")
	}
	if _, ok := e.foundation(record.BasedOn); !ok {
		return Preset{}, jsonrpc.Failed(ReasonUnknownPreset, "there is no built-in preset %q", record.BasedOn)
	}
	if _, err := e.compose(record); err != nil {
		var rpcErr *jsonrpc.Error
		if !errors.As(err, &rpcErr) {
			err = jsonrpc.Failed(ReasonInvalidValue, "%v", err)
		}
		return Preset{}, err
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	if taken, err := e.nameTaken(record.Name, record.ID); err != nil {
		return Preset{}, err
	} else if taken {
		return Preset{}, jsonrpc.Failed(ReasonPresetNameTaken, "there is a preset named %q already", record.Name)
	}
	if err := e.db.Update(func(tx *bbolt.Tx) error {
		if record.ID == "" {
			record.ID, record.AddedAt = newID(), e.now().UnixMilli()
		} else {
			var old copyRecord
			found, err := getJSON(tx, bucketPresets, []byte(record.ID), &old)
			if err != nil {
				return err
			}
			if !found {
				return jsonrpc.Failed(ReasonNotFound, "there is no preset %q", record.ID)
			}
			record.AddedAt = old.AddedAt
		}
		return putJSON(tx, bucketPresets, []byte(record.ID), record)
	}); err != nil {
		return Preset{}, err
	}
	e.announcePresets()
	return record.view(), nil
}

// RemovePreset deletes a user's preset; a connection that used it goes back to its base, or to the standard
// preset.
func (e *Engine) RemovePreset(id string) error {
	e.writeMu.Lock()
	var record copyRecord
	var found bool
	err := e.db.Update(func(tx *bbolt.Tx) error {
		var err error
		if found, err = getJSON(tx, bucketPresets, []byte(id), &record); err != nil || !found {
			return err
		}
		return tx.Bucket(bucketPresets).Delete([]byte(id))
	})
	e.writeMu.Unlock()
	if err != nil {
		return err
	}
	if !found {
		return jsonrpc.Failed(ReasonNotFound, "there is no preset %q", id)
	}
	e.announcePresets()
	if settings, err := e.Settings(); err == nil && settings.Preset == id {
		settings.Preset = cmp.Or(record.BasedOn, standardPreset)
		_, err = e.SetSettings(settings)
		return err
	}
	return nil
}

func (e *Engine) announcePresets() {
	if presets, err := e.Presets(); err == nil {
		e.notify(EventPresetsChanged, presets)
	}
}

const presetLinkPrefix = "poros://route/"

// presetLink is what a poros://route/ link carries: the copy without its ID.
type presetLink struct {
	Kind    string        `json:"kind"`
	Version int           `json:"v"`
	BasedOn string        `json:"based_on"`
	Name    string        `json:"name"`
	Sites   []PresetEntry `json:"sites,omitempty"`
	Apps    []PresetEntry `json:"apps,omitempty"`
	DNS     string        `json:"dns,omitempty"`
}

const presetLinkKind = "poros-preset"

// ExportPreset writes a preset as a link to paste into another Poros.
func (e *Engine) ExportPreset(id string) (PresetLink, error) {
	c, err := e.PresetCopy(id)
	if err != nil {
		return PresetLink{}, err
	}
	data, err := json.Marshal(presetLink{
		Kind: presetLinkKind, Version: 1, BasedOn: c.BasedOn, Name: c.Name, Sites: c.Sites, Apps: c.Apps, DNS: c.DNS,
	})
	if err != nil {
		return PresetLink{}, err
	}
	return PresetLink{Link: presetLinkPrefix + base64.RawURLEncoding.EncodeToString(data)}, nil
}

// nameTaken tells whether a preset other than id, built-in or the user's, has the name; case does not count.
func (e *Engine) nameTaken(name, id string) (bool, error) {
	for _, record := range e.presets {
		if strings.EqualFold(record.Preset.Name, name) {
			return true, nil
		}
	}
	copies, err := e.copies()
	if err != nil {
		return false, err
	}
	for _, c := range copies {
		if c.ID != id && strings.EqualFold(strings.TrimSpace(c.Name), name) {
			return true, nil
		}
	}
	return false, nil
}

// ImportPreset makes a new copy from a link; a name that is taken gets a number: "Games 2".
func (e *Engine) ImportPreset(p PresetLink) (Preset, error) {
	link, err := decodePresetLink(p.Link)
	if err != nil {
		return Preset{}, err
	}
	name := strings.TrimSpace(link.Name)
	for n := 2; name != ""; n++ {
		taken, err := e.nameTaken(name, "")
		if err != nil {
			return Preset{}, err
		}
		if !taken {
			break
		}
		name = fmt.Sprintf("%s %d", strings.TrimSpace(link.Name), n)
	}
	return e.SavePreset(PresetCopy{BasedOn: link.BasedOn, Name: name, Sites: link.Sites, Apps: link.Apps, DNS: link.DNS})
}

func decodePresetLink(text string) (presetLink, error) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(strings.ToLower(text), presetLinkPrefix) {
		return presetLink{}, jsonrpc.Failed(ReasonInvalidLink, "a preset link starts with %s", presetLinkPrefix)
	}
	encoded := strings.TrimRight(text[len(presetLinkPrefix):], "=")
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return presetLink{}, jsonrpc.Failed(ReasonInvalidLink, "the preset link is damaged")
	}
	var link presetLink
	if err = json.Unmarshal(data, &link); err != nil || link.Kind != presetLinkKind || link.Version != 1 {
		return presetLink{}, jsonrpc.Failed(ReasonInvalidLink, "the link holds no Poros preset")
	}
	return link, nil
}
