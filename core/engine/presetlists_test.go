package engine

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestReadPresetLists(t *testing.T) {
	read, err := ReadPresetLists(PresetLists{Lists: map[string]string{
		ActionProxy: "video.example.com\r\n# work\n\n  https://Mail.Example.org/inbox  \nnot a site\nDOMAIN:media.example.net\nfull:exact.example\nvideo.example.com",
		ActionDirect: "192.168.50.0/24\nip:203.0.113.7\nsteam.exe\nC:\\Games\\game.exe\n/usr/bin/firefox\nprocessName:steam\nprocesspath:D:\\Tools\\tool\n" +
			"*.video.example.com\nSTEAM.EXE",
		ActionBlock: "*.ads.example\nsuffix:tracker.example\ngeosite:category-ads\nregex:\\.ads\\.example$\n*.\nsteam.exe",
	}})
	if err != nil {
		t.Fatal(err)
	}
	wantSites := []PresetEntry{
		{Value: "video.example.com", Action: ActionProxy},
		{Value: "https://Mail.Example.org/inbox", Action: ActionProxy},
		{Value: "media.example.net", Action: ActionProxy},
		{Value: "exact.example", Action: ActionProxy},
		{Value: "192.168.50.0/24", Action: ActionDirect},
		{Value: "203.0.113.7", Action: ActionDirect},
		{Value: "*.ads.example", Action: ActionBlock},
		{Value: "tracker.example", Action: ActionBlock},
	}
	wantApps := []PresetEntry{
		{Value: "steam.exe", Action: ActionDirect},
		{Value: `C:\Games\game.exe`, Action: ActionDirect},
		{Value: "/usr/bin/firefox", Action: ActionDirect},
		{Value: "steam", Action: ActionDirect},
		{Value: `D:\Tools\tool`, Action: ActionDirect},
	}
	// The same site in another spelling on another tab is a problem; on its own tab it counts once.
	wantProblems := []PresetListProblem{
		{Action: ActionProxy, Line: 5, Text: "not a site", Reason: ReasonInvalidEntry},
		{Action: ActionDirect, Line: 8, Text: "*.video.example.com", Reason: ReasonDuplicate, Other: ActionProxy},
		{Action: ActionBlock, Line: 3, Text: "geosite:category-ads", Reason: ReasonInvalidEntry},
		{Action: ActionBlock, Line: 4, Text: `regex:\.ads\.example$`, Reason: ReasonInvalidEntry},
		{Action: ActionBlock, Line: 5, Text: "*.", Reason: ReasonInvalidEntry},
		{Action: ActionBlock, Line: 6, Text: "steam.exe", Reason: ReasonDuplicate, Other: ActionDirect},
	}
	if !reflect.DeepEqual(read.Sites, wantSites) {
		t.Errorf("sites:\n%+v\nwant\n%+v", read.Sites, wantSites)
	}
	if !reflect.DeepEqual(read.Apps, wantApps) {
		t.Errorf("apps:\n%+v\nwant\n%+v", read.Apps, wantApps)
	}
	if !reflect.DeepEqual(read.Problems, wantProblems) {
		t.Errorf("problems:\n%+v\nwant\n%+v", read.Problems, wantProblems)
	}

	if _, err = ReadPresetLists(PresetLists{Lists: map[string]string{"warp": "example.com"}}); reason(err) != ReasonInvalidValue {
		t.Errorf("an unknown list: %v", err)
	}
	empty, err := ReadPresetLists(PresetLists{})
	if err != nil || empty.Sites == nil || empty.Apps == nil || empty.Problems == nil {
		t.Errorf("no lists: %+v, %v", empty, err)
	}
}

// The window shows a preset's entries and gives them back unchanged, and what it gives is a preset to save.
func TestPresetListsRoundTrip(t *testing.T) {
	entries := PresetEntries{
		Sites: []PresetEntry{
			{Value: "example.org", Action: ActionBlock},
			{Value: "192.168.50.0/24", Action: ActionDirect},
			{Value: "https://video.example.com/watch", Action: ActionProxy},
			{Value: "setup.exe", Action: ActionProxy},
		},
		Apps: []PresetEntry{
			{Value: "steam.exe", Action: ActionDirect},
			{Value: "steam", Action: ActionDirect},
			{Value: "example.com", Action: ActionProxy},
			{Value: `C:\Games\game.exe`, Action: ActionBlock},
		},
	}
	lists, err := WritePresetLists(entries)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		ActionProxy:  "https://video.example.com/watch\ndomain:setup.exe\nprocessName:example.com",
		ActionDirect: "192.168.50.0/24\nsteam.exe\nprocessName:steam",
		ActionBlock:  "example.org\nC:\\Games\\game.exe",
	}
	if !reflect.DeepEqual(lists.Lists, want) {
		t.Fatalf("lists:\n%q\nwant\n%q", lists.Lists, want)
	}
	read, err := ReadPresetLists(lists)
	if err != nil || len(read.Problems) != 0 {
		t.Fatalf("read back: %+v, %v", read, err)
	}
	// The order changes to that of the tabs, the entries stay.
	wantSites := []PresetEntry{entries.Sites[2], entries.Sites[3], entries.Sites[1], entries.Sites[0]}
	wantApps := []PresetEntry{entries.Apps[2], entries.Apps[0], entries.Apps[1], entries.Apps[3]}
	if !reflect.DeepEqual(read.Sites, wantSites) || !reflect.DeepEqual(read.Apps, wantApps) {
		t.Errorf("read back:\n%+v\n%+v", read.Sites, read.Apps)
	}

	e, _ := openTest(t, t.TempDir())
	draft, _ := e.PresetCopy("all")
	draft.Name, draft.Sites, draft.Apps = "Lists", read.Sites, read.Apps
	if saved, err := e.SavePreset(draft); err != nil || saved.Sites != 4 || saved.Apps != 4 {
		t.Errorf("SavePreset: %+v, %v", saved, err)
	}

	// An entry the window cannot read stays as typed, for the window to name.
	lists, err = WritePresetLists(PresetEntries{Sites: []PresetEntry{{Value: "not a site", Action: ActionProxy}}})
	if err != nil || lists.Lists[ActionProxy] != "not a site" {
		t.Errorf("a broken entry: %q, %v", lists.Lists, err)
	}
	if _, err = WritePresetLists(PresetEntries{Apps: []PresetEntry{{Value: "steam.exe", Action: "elsewhere"}}}); reason(err) != ReasonInvalidEntry {
		t.Errorf("an unknown action: %v", err)
	}

	result, err := e.Handlers()[MethodPresetsReadLists](t.Context(), json.RawMessage(`{"lists":{"block":"ads.example"}}`))
	if read, ok := result.(PresetListsRead); err != nil || !ok || !reflect.DeepEqual(read.Sites, []PresetEntry{{Value: "ads.example", Action: ActionBlock}}) {
		t.Errorf("through JSON-RPC: %+v, %v", result, err)
	}
}
