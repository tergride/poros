package engine

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/sagernet/bbolt"
)

// An import under a name of the user's preset offers both: to take its place, with its ID, choice and place in the
// list, or to be added under a name with a number.
func TestPresetOfATakenName(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	first, err := e.SavePreset(PresetCopy{BasedOn: "all", Name: "Media", DNS: "tls://1.1.1.1", Default: ActionBlock,
		Rules: []PresetRule{rule(CondSuffix, "old.example", ActionBlock)}})
	if err != nil {
		t.Fatal(err)
	}
	last, err := e.SavePreset(PresetCopy{Name: "Work", Rules: []PresetRule{rule(CondSuffix, "work.example", ActionDirect)}})
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.Preset = first.ID
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	file := func(name string, advanced bool, rules ...PresetRule) string {
		data, err := json.Marshal(presetLink{Kind: presetLinkKind, Version: 1, Name: name, Advanced: advanced, Rules: linkRules(rules), Default: ActionDirect})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	fresh, err := e.ReadPreset(PresetLink{Link: file(" Games ", false)})
	if err != nil || fresh.Same != nil || fresh.NewName != "Games" {
		t.Fatalf("a free name: %+v, %v", fresh, err)
	}
	update := file(" media ", true, rule(CondPort, "443", ActionProxy), rule(CondSuffix, "new.example", ActionProxy))
	read, err := e.ReadPreset(PresetLink{Link: update})
	if err != nil || read.Same == nil || read.Same.ID != first.ID || read.NewName != "media 2" || len(read.Preset.Rules) != 2 || !read.Preset.Advanced {
		t.Fatalf("a taken name: %+v, %v", read, err)
	}
	if list, _ := e.Presets(); len(list) != 4 {
		t.Fatalf("reading added a preset: %+v", list)
	}

	replaced, err := e.ReplacePreset(PresetReplace{ID: first.ID, Link: update})
	if err != nil || replaced.ID != first.ID || replaced.Name != "media" || !replaced.Advanced || replaced.Rules != 2 || replaced.BasedOn != "" {
		t.Fatalf("ReplacePreset: %+v, %v", replaced, err)
	}
	stored, err := e.PresetCopy(first.ID)
	if err != nil || stored.DNS != "" || stored.Default != ActionDirect || !reflect.DeepEqual(values(stored.Rules), []string{"port:443", "suffix:new.example"}) {
		t.Errorf("something of the old preset stayed: %+v, %v", stored, err)
	}
	if list, _ := e.Presets(); len(list) != 4 || list[2].ID != first.ID || list[3].ID != last.ID {
		t.Errorf("the replaced preset moved: %+v", list)
	}
	if now, _ := e.Settings(); now.Preset != first.ID {
		t.Errorf("the replaced preset is no longer chosen: %q", now.Preset)
	}

	// An advanced preset does not become simple, a replaced one neither.
	simple := file("Media", false, rule(CondSuffix, "simple.example", ActionProxy))
	if kept, err := e.ReplacePreset(PresetReplace{ID: first.ID, Link: simple}); err != nil || !kept.Advanced || kept.Rules != 1 || kept.Name != "Media" {
		t.Errorf("a simple preset in place of an advanced one: %+v, %v", kept, err)
	}
	if grown, err := e.ReplacePreset(PresetReplace{ID: last.ID, Link: file("work", true, rule(CondPort, "22", ActionDirect))}); err != nil || !grown.Advanced {
		t.Errorf("an advanced preset in place of a simple one: %+v, %v", grown, err)
	}

	directDNS, err := json.Marshal(presetLink{Kind: presetLinkKind, Version: 1, Name: "Media", DirectDNS: "tls://1.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := e.PresetCopy(first.ID)
	for _, c := range []struct {
		id, link, reason string
	}{
		{"gone", simple, ReasonNotFound},
		{"all", file("All traffic", false), ReasonNotFound},
		{last.ID, simple, ReasonNotFound},
		{first.ID, "poros://route/%%%", ReasonInvalidLink},
		{first.ID, file("Media", true, rule(CondPort, "0", ActionProxy)), ReasonInvalidEntry},
		// What an import refuses does not come in over an advanced preset either.
		{first.ID, file("Media", false, rule(CondPort, "443", ActionProxy)), ReasonAdvancedOnly},
		{first.ID, string(directDNS), ReasonAdvancedOnly},
	} {
		if _, err := e.ReplacePreset(PresetReplace{ID: c.id, Link: c.link}); reason(err) != c.reason {
			t.Errorf("replace %q by %q: %v, want %s", c.id, c.link, err, c.reason)
		}
	}
	if after, _ := e.PresetCopy(first.ID); !reflect.DeepEqual(before, after) {
		t.Errorf("a refused replacement changed the preset: %+v", after)
	}

	// A built-in preset has nothing to replace: an import under its name gets a number, as before.
	if builtin, err := e.ReadPreset(PresetLink{Link: file("all TRAFFIC", false)}); err != nil || builtin.Same != nil || builtin.NewName != "all TRAFFIC 2" {
		t.Errorf("the name of a built-in preset: %+v, %v", builtin, err)
	}
	if added, err := e.ImportPreset(PresetLink{Link: simple}); err != nil || added.ID == first.ID || added.Name != "Media 2" {
		t.Errorf("a new one instead: %+v, %v", added, err)
	}
	if third, err := e.ReadPreset(PresetLink{Link: simple}); err != nil || third.Same == nil || third.Same.ID != first.ID || third.NewName != "Media 3" {
		t.Errorf("the third of the name: %+v, %v", third, err)
	}
}

// A preset renamed while its replacement waits for the write is not the one the user agreed to lose.
func TestReplacePresetChecksTheNameAsItWrites(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	saved, err := e.SavePreset(PresetCopy{Name: "Media"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(presetLink{Kind: presetLinkKind, Version: 1, Name: "Media", Default: ActionBlock})
	if err != nil {
		t.Fatal(err)
	}
	e.writeMu.Lock()
	replaced := make(chan error, 1)
	go func() {
		_, err := e.ReplacePreset(PresetReplace{ID: saved.ID, Link: string(data)})
		replaced <- err
	}()
	// Long enough for the replacement to come to the lock.
	time.Sleep(100 * time.Millisecond)
	err = e.db.Update(func(tx *bbolt.Tx) error {
		var record copyRecord
		if _, err := getJSON(tx, bucketPresets, []byte(saved.ID), &record); err != nil {
			return err
		}
		record.Name = "Renamed"
		return putJSON(tx, bucketPresets, []byte(saved.ID), record)
	})
	e.writeMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := <-replaced; reason(err) != ReasonNotFound {
		t.Errorf("replace a renamed preset: %v, want %s", err, ReasonNotFound)
	}
	if now, err := e.PresetCopy(saved.ID); err != nil || now.Name != "Renamed" || now.Default == ActionBlock {
		t.Errorf("the renamed preset was replaced: %+v, %v", now, err)
	}
}
