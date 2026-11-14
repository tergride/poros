package engine

import (
	"slices"
	"testing"

	"PorosCore/config"
)

// WARP in the settings: the keys of each mode are data, kept while WARP is off; on, the device of its mode must be
// whole. Rules to WARP belong to an advanced preset and have a tab of the lists window there.
func TestWARPSettings(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	settings, err := e.Settings()
	if err != nil || settings.WARP || settings.WARPMode != WARPModeWireGuard {
		t.Fatalf("the defaults: %+v, %v", settings, err)
	}
	device := WARPDevice{
		Endpoint: "162.159.192.1:2408", PrivateKey: "yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=",
		PeerKey: "xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=", Addresses: "172.16.0.2/32", Reserved: "12, 85, 201",
	}
	partial := settings
	partial.WARPWireGuard = WARPDevice{Endpoint: "162.159.192.1:2408"}
	if _, err := e.SetSettings(partial); err != nil {
		t.Errorf("a device in the making while WARP is off: %v", err)
	}
	for name, change := range map[string]func(s *Settings){
		"on without a device": func(s *Settings) { s.WARP = true },
		"a mode":              func(s *Settings) { s.WARPMode = "amnezia" },
		"reserved bytes":      func(s *Settings) { s.WARPWireGuard = device; s.WARPWireGuard.Reserved = "1, 2" },
		"a MASQUE key":        func(s *Settings) { s.WARPMASQUE = WARPDevice{PrivateKey: device.PrivateKey} },
	} {
		wrong := settings
		change(&wrong)
		if _, err := e.SetSettings(wrong); reason(err) != ReasonInvalidValue {
			t.Errorf("%s: %v", name, err)
		}
	}
	on := settings
	on.WARP, on.WARPWireGuard = true, device
	if _, err := e.SetSettings(on); err != nil {
		t.Fatal(err)
	}
	if w := warpOf(on); w == nil || w.Mode != config.WARPWireGuard || w.Port != 2408 || !slices.Equal(w.Reserved, []uint8{12, 85, 201}) {
		t.Errorf("the WARP of a connection: %+v", w)
	}
	if w := warpOf(settings); w != nil {
		t.Errorf("WARP off: %+v", w)
	}
	if _, err := e.RegisterWARP(t.Context(), WARPRegister{Mode: "amnezia"}); reason(err) != ReasonInvalidValue {
		t.Errorf("an unknown mode: %v", err)
	}

	ai := rule(CondSuffix, "chatgpt.com", ActionWARP)
	if _, err := CheckPresetRules(PresetRules{Rules: []PresetRule{ai}}); problemOf(t, err).Reason != ReasonAdvancedOnly {
		t.Errorf("a simple preset has no WARP: %v", err)
	}
	saved, err := e.SavePreset(PresetCopy{BasedOn: "all", Name: "AI", Advanced: true, Rules: []PresetRule{ai}})
	if err != nil {
		t.Fatal(err)
	}
	if preset, _, _ := e.preset(saved.ID); preset.Rules[1].Outbound != config.OutboundWARP {
		t.Errorf("a rule to WARP: %+v", preset.Rules)
	}
	lists, err := WritePresetLists(PresetRules{Advanced: true, Rules: []PresetRule{ai}})
	if err != nil || lists.Lists[ActionWARP] != "suffix:chatgpt.com" {
		t.Errorf("the WARP tab: %+v, %v", lists, err)
	}
	read, err := ReadPresetLists(PresetListsInput{Advanced: true, Lists: map[string]string{ActionWARP: "suffix:claude.ai"}})
	if err != nil || len(read.Rules) != 1 || read.Rules[0].Action != ActionWARP {
		t.Errorf("a line of the WARP tab: %+v, %v", read, err)
	}
	if _, err := ReadPresetLists(PresetListsInput{Lists: map[string]string{ActionWARP: ""}}); reason(err) != ReasonInvalidValue {
		t.Errorf("a simple preset has no WARP tab: %v", err)
	}
}
