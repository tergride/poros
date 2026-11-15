package engine

import (
	"bytes"
	"cmp"
	"compress/zlib"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/sagernet/bbolt"

	"PorosCore/config"
	"PorosCore/internal/jsonrpc"
)

//go:embed presets/*.json
var presetFiles embed.FS

// The built-in presets in the order they are shown: everything, then the hardened one.
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

// copyRecord is a user's preset as stored: its own rules in the order they are checked. BasedOn is the built-in
// preset it is a copy of, "" for one made from scratch.
type copyRecord struct {
	ID        string       `json:"id"`
	BasedOn   string       `json:"basedOn"`
	Name      string       `json:"name"`
	Advanced  bool         `json:"advanced"`
	Rules     []PresetRule `json:"rules"`
	DNS       string       `json:"dns"`
	DirectDNS string       `json:"directDns"`
	Default   string       `json:"default,omitempty"`
	AddedAt   int64        `json:"addedAt"`
}

func (r copyRecord) view() Preset {
	return Preset{ID: r.ID, Name: r.Name, BasedOn: r.BasedOn, Advanced: r.Advanced, Rules: len(r.Rules)}
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

func (e *Engine) copyRecord(id string) (copyRecord, bool, error) {
	var record copyRecord
	var found bool
	err := e.db.View(func(tx *bbolt.Tx) error {
		var err error
		found, err = getJSON(tx, bucketPresets, []byte(id), &record)
		return err
	})
	return record, found, err
}

// Presets lists the built-in presets, then the user's copies in the order they were made.
func (e *Engine) Presets() ([]Preset, error) {
	copies, err := e.copies()
	if err != nil {
		return nil, err
	}
	views := make([]Preset, 0, len(e.presets)+len(copies))
	for _, record := range e.presets {
		views = append(views, Preset{ID: record.ID, Name: record.Preset.Name, Builtin: true, Rules: len(record.Preset.Rules)})
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
	record, found, err := e.copyRecord(id)
	if err != nil || !found {
		return config.Preset{}, false, err
	}
	preset, err := e.compose(record)
	return preset, err == nil, err
}

// head counts the rules of a built-in preset that come before the user's: those of DNS queries.
func head(base config.Preset) int {
	n := 0
	for n < len(base.Rules) && base.Rules[n].Action == config.ActionHijackDNS {
		n++
	}
	return n
}

// compose puts a copy together: the DNS rules of its base first, then the user's rules that are on, then the rest of
// the base. The first rule that matches decides, so the user's rules win over the base.
func (e *Engine) compose(record copyRecord) (config.Preset, error) {
	base, ok := e.foundation(record.BasedOn)
	if !ok {
		return config.Preset{}, fmt.Errorf("the copy is based on %q, which is no built-in preset", record.BasedOn)
	}
	preset := base.Preset
	preset.Name, preset.DNS = record.Name, record.DNS
	if record.Advanced {
		preset.DirectDNS = record.DirectDNS
	}
	if record.Default != "" {
		preset.DefaultOutbound = record.Default
	}
	var own []config.Rule
	for _, rule := range record.Rules {
		if !rule.Off {
			own = append(own, configRule(rule, ""))
		}
	}
	n := head(base.Preset)
	preset.Rules = slices.Concat(base.Preset.Rules[:n], own, base.Preset.Rules[n:])
	return preset, preset.Validate()
}

func nonNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}

func inherited(base config.Preset) []InheritedRule {
	n := head(base)
	rules := []InheritedRule{}
	for i, rule := range base.Rules {
		inherited := InheritedRule{Name: rule.Name, Action: rule.Outbound, First: i < n}
		switch rule.Action {
		case config.ActionHijackDNS:
			inherited.Action = "dns"
		case config.ActionReject:
			inherited.Action = ActionBlock
		}
		rules = append(rules, inherited)
	}
	return rules
}

// PresetCopy gives a preset for the editor: a user's one, or a built-in one to show, with Builtin set.
func (e *Engine) PresetCopy(id string) (PresetCopy, error) {
	if base, ok := e.builtin(id); ok {
		return PresetCopy{
			ID: id, Builtin: true, Name: base.Preset.Name, Rules: []PresetRule{}, Default: base.Preset.DefaultOutbound,
			Inherited: inherited(base.Preset),
		}, nil
	}
	record, found, err := e.copyRecord(id)
	if err != nil {
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
		ID: record.ID, BasedOn: record.BasedOn, Name: record.Name, Advanced: record.Advanced,
		Rules: nonNil(record.Rules), DNS: record.DNS, DirectDNS: record.DirectDNS,
		Default: cmp.Or(record.Default, base.Preset.DefaultOutbound), Inherited: inherited(base.Preset),
	}, nil
}

// SavePreset stores a user's preset, a new one when it has no ID. An advanced preset stays advanced.
func (e *Engine) SavePreset(c PresetCopy) (Preset, error) {
	if c.Builtin {
		return Preset{}, jsonrpc.Failed(ReasonInvalidValue, "a built-in preset does not change")
	}
	record := copyRecord{
		ID: c.ID, BasedOn: c.BasedOn, Name: strings.TrimSpace(c.Name), Advanced: c.Advanced,
		DNS: strings.TrimSpace(c.DNS), DirectDNS: strings.TrimSpace(c.DirectDNS), Default: c.Default,
	}
	if record.Name == "" {
		return Preset{}, jsonrpc.Failed(ReasonInvalidValue, "the preset needs a name")
	}
	if !slices.Contains([]string{"", ActionProxy, ActionDirect, ActionBlock}, record.Default) {
		return Preset{}, jsonrpc.Failed(ReasonInvalidValue, "the default must be proxy, direct or block, not %q", record.Default)
	}
	if !record.Advanced && record.DirectDNS != "" {
		return Preset{}, jsonrpc.Failed(ReasonAdvancedOnly, "a simple preset has no direct DNS")
	}
	if _, ok := e.foundation(record.BasedOn); !ok {
		return Preset{}, jsonrpc.Failed(ReasonUnknownPreset, "there is no built-in preset %q", record.BasedOn)
	}
	rules, err := checkRules(ruleSetOf(record.Advanced, false), c.Rules)
	if err != nil {
		return Preset{}, err
	}
	record.Rules = rules
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
			return putJSON(tx, bucketPresets, []byte(record.ID), record)
		}
		var old copyRecord
		found, err := getJSON(tx, bucketPresets, []byte(record.ID), &old)
		if err != nil {
			return err
		}
		if !found {
			return jsonrpc.Failed(ReasonNotFound, "there is no preset %q", record.ID)
		}
		if old.Advanced && !record.Advanced {
			return jsonrpc.Failed(ReasonInvalidValue, "an advanced preset does not become simple")
		}
		record.AddedAt = old.AddedAt
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
	// The choice moves on under the same writeMu: outside it, settings read before a server went would fail their
	// check, and the preset that is gone would stay chosen.
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	var record copyRecord
	var found bool
	err := e.db.Update(func(tx *bbolt.Tx) error {
		var err error
		if found, err = getJSON(tx, bucketPresets, []byte(id), &record); err != nil || !found {
			return err
		}
		return tx.Bucket(bucketPresets).Delete([]byte(id))
	})
	if err != nil {
		return err
	}
	if !found {
		return jsonrpc.Failed(ReasonNotFound, "there is no preset %q", id)
	}
	e.announcePresets()
	if settings, err := e.Settings(); err == nil && settings.Preset == id {
		settings.Preset = cmp.Or(record.BasedOn, standardPreset)
		_, err = e.setSettings(settings)
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

// presetLink is what a poros://route/ link carries: the copy without its ID, as JSON compressed with zlib. What is
// unset stays out, so that a preset of dozens of rules takes a few hundred characters: short enough to paste into a
// message and to read from a QR code on a picture.
type presetLink struct {
	Kind      string     `json:"kind"`
	Version   int        `json:"v"`
	BasedOn   string     `json:"based_on,omitempty"`
	Name      string     `json:"name"`
	Advanced  bool       `json:"advanced,omitempty"`
	Rules     []linkRule `json:"rules,omitempty"`
	DNS       string     `json:"dns,omitempty"`
	DirectDNS string     `json:"direct_dns,omitempty"`
	Default   string     `json:"default,omitempty"`
}

// linkRule is a PresetRule as a link carries it, without the fields the rule leaves unset.
type linkRule struct {
	Name       string          `json:"name,omitempty"`
	Conditions []RuleCondition `json:"conditions"`
	Invert     bool            `json:"invert,omitempty"`
	Off        bool            `json:"off,omitempty"`
	Action     string          `json:"action"`
}

func linkRules(rules []PresetRule) []linkRule {
	out := make([]linkRule, len(rules))
	for i, r := range rules {
		out[i] = linkRule(r)
	}
	return out
}

func (l presetLink) rules() []PresetRule {
	out := make([]PresetRule, len(l.Rules))
	for i, r := range l.Rules {
		out[i] = PresetRule(r)
	}
	return out
}

const presetLinkKind = "poros-preset"

// ExportPreset writes a preset as a link to paste into another Poros.
func (e *Engine) ExportPreset(id string) (PresetLink, error) {
	c, err := e.PresetCopy(id)
	if err != nil {
		return PresetLink{}, err
	}
	if c.Builtin {
		return PresetLink{}, jsonrpc.Failed(ReasonInvalidValue, "a built-in preset is in every Poros already")
	}
	link, err := encodePresetLink(presetLink{
		Kind: presetLinkKind, Version: 1, BasedOn: c.BasedOn, Name: c.Name, Advanced: c.Advanced, Rules: linkRules(c.Rules),
		DNS: c.DNS, DirectDNS: c.DirectDNS, Default: c.Default,
	})
	return PresetLink{Link: link}, err
}

// ExportPresetFile writes a preset as a file of JSON, what its link holds, under the preset's name.
func (e *Engine) ExportPresetFile(id string) (PresetFile, error) {
	exported, err := e.ExportPreset(id)
	if err != nil {
		return PresetFile{}, err
	}
	link, err := decodePresetLink(exported.Link)
	if err != nil {
		return PresetFile{}, err
	}
	data, err := json.MarshalIndent(link, "", "  ")
	if err != nil {
		return PresetFile{}, err
	}
	return PresetFile{Name: fileName(link.Name) + ".json", Text: string(data) + "\n"}, nil
}

// fileName makes a name fit for a file on every system: the characters Windows refuses become «_», and the dots and
// spaces it drops at the end go.
func fileName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(strings.TrimSpace(name), ". ")
	if name == "" {
		return "preset"
	}
	return name
}

func encodePresetLink(link presetLink) (string, error) {
	data, err := json.Marshal(link)
	if err != nil {
		return "", err
	}
	var packed bytes.Buffer
	w, err := zlib.NewWriterLevel(&packed, zlib.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err = w.Write(data); err != nil {
		return "", err
	}
	if err = w.Close(); err != nil {
		return "", err
	}
	return presetLinkPrefix + base64.RawURLEncoding.EncodeToString(packed.Bytes()), nil
}

// maxPresetJSON bounds what a link may unpack to: a preset of thousands of rules is far less, and a small link that
// unpacks to gigabytes stops there.
const maxPresetJSON = 1 << 20

func unpackPresetLink(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	unpacked, err := io.ReadAll(io.LimitReader(r, maxPresetJSON+1))
	if err == nil && len(unpacked) > maxPresetJSON {
		err = errors.New("the preset is too large")
	}
	return unpacked, err
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

// ImportPreset makes a new copy from a link or the text of a preset file; a name that is taken gets a number:
// "Games 2".
func (e *Engine) ImportPreset(p PresetLink) (Preset, error) {
	link, err := decodePresetText(p.Link)
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
	return e.SavePreset(link.copy(name))
}

// copy is the preset a link brings, under name.
func (l presetLink) copy(name string) PresetCopy {
	return PresetCopy{
		BasedOn: l.BasedOn, Name: name, Advanced: l.Advanced, Rules: l.rules(), DNS: l.DNS,
		DirectDNS: l.DirectDNS, Default: l.Default,
	}
}

func decodePresetLink(text string) (presetLink, error) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(strings.ToLower(text), presetLinkPrefix) {
		return presetLink{}, jsonrpc.Failed(ReasonInvalidLink, "a preset link starts with %s", presetLinkPrefix)
	}
	encoded := strings.TrimRight(text[len(presetLinkPrefix):], "=")
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	// Plain JSON starts with «{», never the first byte of zlib: such a link reads as it is.
	if err == nil && !bytes.HasPrefix(data, []byte("{")) {
		data, err = unpackPresetLink(data)
	}
	if err != nil {
		return presetLink{}, jsonrpc.Failed(ReasonInvalidLink, "the preset link is damaged")
	}
	return readPresetJSON(data)
}

// readPresetJSON reads what a link or a preset file holds. Unknown keys are refused: a rule that lost a condition
// would match more.
func readPresetJSON(data []byte) (presetLink, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var link presetLink
	if err := decoder.Decode(&link); err != nil || link.Kind != presetLinkKind || link.Version != 1 {
		return presetLink{}, jsonrpc.Failed(ReasonInvalidLink, "it holds no Poros preset")
	}
	return link, nil
}

// decodePresetText reads a poros://route/ link, or the text of a preset file, which is JSON.
func decodePresetText(text string) (presetLink, error) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "{") {
		return decodePresetLink(text)
	}
	if len(text) > maxPresetJSON {
		return presetLink{}, jsonrpc.Failed(ReasonInvalidLink, "the preset file is larger than %d KB", maxPresetJSON>>10)
	}
	return readPresetJSON([]byte(text))
}
