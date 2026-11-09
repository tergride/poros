package engine

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

// Every line says what it is by its prefix, as in Throne: nothing is guessed.
func TestReadPresetLists(t *testing.T) {
	lists := map[string]string{
		ActionProxy: "suffix:video.example.com\r\n# work\n\n  suffix:https://Mail.Example.org/inbox  \nmedia.example.net\nSUFFIX:media.example.net\n" +
			"domain:exact.example\nsuffix:video.example.com",
		ActionDirect: "ip:192.168.50.0/24\nIP:203.0.113.7\nprocessName:steam.exe\nprocessPath:C:\\Games\\game.exe\nprocessname:steam\n" +
			"suffix:*.video.example.com\nprocessName:STEAM.EXE",
		ActionBlock: "suffix:*.ads.example\ngeosite:category-ads\nregex:\\.ads\\.example$\nsuffix:*.\nprocessName:steam.exe\nC:\\Tools\\tool.exe",
	}
	read, err := ReadPresetLists(PresetListsInput{Lists: lists})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"suffix:video.example.com", "suffix:mail.example.org", "suffix:media.example.net",
		"ip:192.168.50.0/24", "ip:203.0.113.7", "processName:steam.exe", `processPath:C:\Games\game.exe`, "suffix:ads.example"}
	if got := values(read.Rules); !slices.Equal(got, want) {
		t.Errorf("rules:\n%q\nwant\n%q", got, want)
	}
	if read.Rules[3].Action != ActionDirect || read.Rules[7].Action != ActionBlock {
		t.Errorf("the tab is the action: %+v", read.Rules)
	}
	// The same rule in another spelling on another tab is a problem; on its own tab it counts once.
	wantProblems := []PresetListProblem{
		{Action: ActionProxy, Line: 5, Text: "media.example.net", Reason: ReasonNoPrefix},
		{Action: ActionProxy, Line: 7, Text: "domain:exact.example", Reason: ReasonAdvancedOnly},
		{Action: ActionDirect, Line: 6, Text: "suffix:*.video.example.com", Reason: ReasonDuplicate, Other: ActionProxy},
		{Action: ActionBlock, Line: 2, Text: "geosite:category-ads", Reason: ReasonNoPrefix},
		{Action: ActionBlock, Line: 3, Text: `regex:\.ads\.example$`, Reason: ReasonAdvancedOnly},
		{Action: ActionBlock, Line: 4, Text: "suffix:*.", Reason: ReasonInvalidEntry},
		{Action: ActionBlock, Line: 5, Text: "processName:steam.exe", Reason: ReasonDuplicate, Other: ActionDirect},
		{Action: ActionBlock, Line: 6, Text: `C:\Tools\tool.exe`, Reason: ReasonNoPrefix},
	}
	if !reflect.DeepEqual(read.Problems, wantProblems) {
		t.Errorf("problems:\n%+v\nwant\n%+v", read.Problems, wantProblems)
	}

	// An advanced preset takes all the kinds.
	advanced, err := ReadPresetLists(PresetListsInput{Advanced: true, Lists: map[string]string{
		ActionDirect: "domain:exact.example\nkeyword:tracker\nregex:\\.ads\\.example$\nprocessPathRegex:(?i)games\nport:8000-8080\nport:443\n" +
			"protocol:QUIC\nnetwork:udp\nipVersion:6\nsourceIP:192.168.1.0/24\nprivate:",
	}})
	want = []string{"domain:exact.example", "keyword:tracker", `regex:\.ads\.example$`, "processPathRegex:(?i)games", "port:8000-8080", "port:443",
		"protocol:quic", "network:udp", "ipVersion:6", "sourceIP:192.168.1.0/24"}
	if got := values(advanced.Rules); err != nil || !slices.Equal(got, want) ||
		!reflect.DeepEqual(advanced.Problems, []PresetListProblem{{Action: ActionDirect, Line: 11, Text: "private:", Reason: ReasonNoPrefix}}) {
		t.Errorf("advanced: %q, %+v, %v", got, advanced.Problems, err)
	}

	if _, err = ReadPresetLists(PresetListsInput{Lists: map[string]string{"warp": "suffix:example.com"}}); reason(err) != ReasonInvalidValue {
		t.Errorf("an unknown list: %v", err)
	}
	empty, err := ReadPresetLists(PresetListsInput{})
	if err != nil || empty.Rules == nil || empty.Problems == nil {
		t.Errorf("no lists: %+v, %v", empty, err)
	}
}

// The window writes the rules it has lines for; after it, a rule with a line keeps its place, its name and its
// switch, one without a line stays, a removed line removes its rule, and a new line comes last before narrower rules
// rise over wider ones.
func TestPresetListsKeepPlaces(t *testing.T) {
	games := PresetRule{Name: "Games", Action: ActionDirect, Conditions: []RuleCondition{
		{Kind: CondProcessName, Values: []string{"game.exe"}}, {Kind: CondNetwork, Values: []string{"udp"}},
	}}
	named := rule(CondSuffix, "a.example", ActionProxy)
	named.Name, named.Off = "A", true
	private := PresetRule{Action: ActionProxy, Conditions: []RuleCondition{{Kind: CondPrivate, Values: []string{}}}}
	inverted := rule(CondSuffix, "c.example", ActionProxy)
	inverted.Invert = true
	rules := []PresetRule{games, named, rule(CondIP, "10.0.0.0/8", ActionDirect), private, inverted, rule(CondSuffix, "b.example", ActionBlock)}

	lists, err := WritePresetLists(PresetRules{Advanced: true, Rules: rules})
	want := map[string]string{ActionProxy: "suffix:a.example", ActionDirect: "ip:10.0.0.0/8", ActionBlock: "suffix:b.example"}
	if err != nil || !reflect.DeepEqual(lists.Lists, want) {
		t.Fatalf("lists: %q, %v", lists.Lists, err)
	}
	if _, err = WritePresetLists(PresetRules{Rules: []PresetRule{rule(CondSuffix, "example.com", "elsewhere")}}); reason(err) != ReasonInvalidValue {
		t.Errorf("an unknown action: %v", err)
	}

	read, err := ReadPresetLists(PresetListsInput{Advanced: true, Rules: rules, Lists: map[string]string{
		ActionProxy:  "suffix:a.example\nsuffix:x.a.example",
		ActionDirect: "port:443",
		ActionBlock:  "ip:10.0.0.0/8",
	}})
	if err != nil || len(read.Problems) != 0 {
		t.Fatalf("read: %+v, %v", read, err)
	}
	wantRules := []PresetRule{games, rule(CondSuffix, "x.a.example", ActionProxy), named, rule(CondIP, "10.0.0.0/8", ActionBlock), private, inverted, rule(CondPort, "443", ActionDirect)}
	if !reflect.DeepEqual(read.Rules, wantRules) {
		t.Errorf("rules:\n%+v\nwant\n%+v", read.Rules, wantRules)
	}

	e, _ := openTest(t, t.TempDir())
	if saved, err := e.SavePreset(PresetCopy{BasedOn: "all", Name: "Lists", Advanced: true, Rules: read.Rules}); err != nil || saved.Rules != 7 {
		t.Errorf("what the window gives is a preset to save: %+v, %v", saved, err)
	}
	result, err := e.Handlers()[MethodPresetsReadLists](t.Context(), json.RawMessage(`{"advanced":false,"rules":[],"lists":{"block":"suffix:ads.example"}}`))
	if read, ok := result.(PresetListsRead); err != nil || !ok || !slices.Equal(values(read.Rules), []string{"suffix:ads.example"}) || read.Rules[0].Action != ActionBlock {
		t.Errorf("through JSON-RPC: %+v, %v", result, err)
	}
}
