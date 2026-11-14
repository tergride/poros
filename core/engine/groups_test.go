package engine

import (
	"reflect"
	"slices"
	"testing"

	"PorosCore/profile"
)

func ids(servers []Server) []string {
	list := make([]string, len(servers))
	for i, s := range servers {
		list[i] = s.ID
	}
	return list
}

func names(servers []Server) []string {
	list := make([]string, len(servers))
	for i, s := range servers {
		list[i] = s.Name
	}
	return list
}

func TestGroupsOfTheUser(t *testing.T) {
	e, events := openTest(t, t.TempDir())
	if _, err := e.AddGroup(AddGroup{Name: "  "}); reason(err) != ReasonInvalidValue {
		t.Fatalf("a group without a name: %v", err)
	}
	mine, err := e.AddGroup(AddGroup{Name: " Mine "})
	if err != nil || mine.Kind != GroupManual || mine.Name != "Mine" || mine.Servers != 0 {
		t.Fatalf("AddGroup: %+v, %v", mine, err)
	}
	if len(*events) != 1 || (*events)[0].method != EventGroupsChanged {
		t.Fatalf("adding a group is announced: %+v", *events)
	}

	p := newProvider(t)
	p.serve(200, vless("de.example.com", "Germany"))
	sub, err := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := e.Groups(); len(list) != 2 || list[0].ID != sub.Group.ID || list[1].ID != mine.ID {
		t.Fatalf("subscriptions come before the user's groups: %+v", list)
	}

	if _, err = e.RenameGroup(RenameGroup{ID: mine.ID}); reason(err) != ReasonInvalidValue {
		t.Fatalf("a group of the user's keeps a name: %v", err)
	}
	if renamed, err := e.RenameGroup(RenameGroup{ID: mine.ID, Name: "Home"}); err != nil || renamed.Name != "Home" {
		t.Fatalf("RenameGroup: %+v, %v", renamed, err)
	}
	if _, err = e.UpdateSubscription(t.Context(), mine.ID); reason(err) != ReasonInvalidValue {
		t.Fatalf("a group of the user's has nothing to update: %v", err)
	}
	if err = e.RemoveGroup(mine.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := e.Groups(); len(list) != 1 {
		t.Fatalf("RemoveGroup left %+v", list)
	}
}

func TestAddServers(t *testing.T) {
	e, events := openTest(t, t.TempDir())
	text := vless("de.example.com", "🇩🇪 Germany") + "\n" + "vless://broken\n" + vless("nl.example.com", "Netherlands")

	if _, err := e.AddServers(AddServers{Text: text}); reason(err) != ReasonInvalidValue {
		t.Fatalf("servers without a group: %v", err)
	}
	if _, err := e.AddServers(AddServers{NewGroup: "Mine", Text: "https://sub.example.com/u/1"}); reason(err) != ReasonSubscriptionURL {
		t.Fatalf("a subscription's link: %v", err)
	}
	if _, err := e.AddServers(AddServers{NewGroup: "Mine", Text: "just a note"}); reason(err) != ReasonNoServers {
		t.Fatalf("a text without servers: %v", err)
	}
	*events = nil
	added, err := e.AddServers(AddServers{NewGroup: " Mine ", Text: text})
	if err != nil || added.Group.Kind != GroupManual || added.Group.Name != "Mine" || added.Group.Servers != 2 || len(added.IDs) != 2 {
		t.Fatalf("AddServers: %+v, %v", added, err)
	}
	if len(added.Skips) != 1 || added.Skips[0].Count != 1 {
		t.Fatalf("the broken line is reported: %+v", added.Skips)
	}
	if len(*events) != 2 || (*events)[0].method != EventGroupsChanged || (*events)[1].params != (ServersChanged{added.Group.ID}) {
		t.Fatalf("events: %+v", *events)
	}
	servers, _ := e.Servers(ServersQuery{GroupID: added.Group.ID})
	if !reflect.DeepEqual(ids(servers), added.IDs) || servers[0].Name != "🇩🇪 Germany" || servers[0].Country != "DE" {
		t.Fatalf("the servers of the text in its order: %+v", servers)
	}

	more, err := e.AddServers(AddServers{GroupID: added.Group.ID, Text: vless("fi.example.com", "Finland")})
	if err != nil || more.Group.Servers != 3 {
		t.Fatalf("adding to the group: %+v, %v", more, err)
	}

	p := newProvider(t)
	p.serve(200, vless("se.example.com", "Sweden"))
	sub, _ := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	if _, err = e.AddServers(AddServers{GroupID: sub.Group.ID, Text: text}); reason(err) != ReasonNotManual {
		t.Fatalf("servers go to a group of the user's only: %v", err)
	}
	if _, err = e.AddServers(AddServers{GroupID: "missing", Text: text}); reason(err) != ReasonNotFound {
		t.Fatalf("a missing group: %v", err)
	}
}

func TestEditServers(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	p := newProvider(t)
	p.serve(200, vless("de.example.com", "Germany"), vless("nl.example.com", "Netherlands"))
	sub, _ := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	mine, _ := e.AddServers(AddServers{NewGroup: "Mine", Text: vless("fi.example.com", "Finland") + "\n" + vless("se.example.com", "Sweden")})
	subServers, _ := e.Servers(ServersQuery{GroupID: sub.Group.ID})

	got, err := e.Server(ByID{ID: mine.IDs[0]})
	if err != nil || got.Server.GroupID != mine.Group.ID || got.Profile.Server != "fi.example.com" || got.Profile.VLESS == nil {
		t.Fatalf("Server: %+v, %v", got, err)
	}
	if _, err = e.Server(ByID{ID: "missing"}); reason(err) != ReasonNotFound {
		t.Fatalf("a missing server: %v", err)
	}

	edited := got.Profile
	edited.Name = "Finland 2"
	saved, err := e.SaveServer(SaveServer{ID: mine.IDs[0], Profile: edited})
	if err != nil || saved.ID != mine.IDs[0] || saved.Name != "Finland 2" {
		t.Fatalf("SaveServer: %+v, %v", saved, err)
	}
	broken := edited
	broken.VLESS = &profile.VLESS{}
	if _, err = e.SaveServer(SaveServer{ID: mine.IDs[0], Profile: broken}); reason(err) != ReasonInvalidValue {
		t.Fatalf("an invalid profile is refused: %v", err)
	}
	if _, err = e.SaveServer(SaveServer{GroupID: sub.Group.ID, Profile: edited}); reason(err) != ReasonNotManual {
		t.Fatalf("a new server in a subscription: %v", err)
	}
	created, err := e.SaveServer(SaveServer{GroupID: mine.Group.ID, Profile: edited})
	if err != nil || created.ID == "" || created.GroupID != mine.Group.ID {
		t.Fatalf("a new server: %+v, %v", created, err)
	}

	copies, err := e.CloneServers(ServerIDs{IDs: []string{mine.IDs[0], subServers[0].ID}})
	if err != nil || len(copies.IDs) != 2 {
		t.Fatalf("CloneServers: %+v, %v", copies, err)
	}
	inMine, _ := e.Servers(ServersQuery{GroupID: mine.Group.ID})
	if want := []string{mine.IDs[0], copies.IDs[0], mine.IDs[1], created.ID}; !reflect.DeepEqual(ids(inMine), want) {
		t.Fatalf("a copy goes right after its server: %v, want %v", ids(inMine), want)
	}
	if inSub, _ := e.Servers(ServersQuery{GroupID: sub.Group.ID}); len(inSub) != 3 || inSub[1].ID != copies.IDs[1] {
		t.Fatalf("a copy in a subscription: %+v", inSub)
	}

	other, _ := e.AddGroup(AddGroup{Name: "Other"})
	moved, err := e.MoveServers(MoveServers{IDs: []string{mine.IDs[1], subServers[1].ID}, GroupID: other.ID})
	if err != nil || len(moved.IDs) != 2 || moved.IDs[0] != mine.IDs[1] || moved.IDs[1] == subServers[1].ID {
		t.Fatalf("MoveServers keeps the ID of a server of the user's and copies one of a subscription: %+v, %v", moved, err)
	}
	inOther, _ := e.Servers(ServersQuery{GroupID: other.ID})
	if !reflect.DeepEqual(names(inOther), []string{"Sweden", "Netherlands"}) {
		t.Fatalf("moved servers: %v", names(inOther))
	}
	if inMine, _ = e.Servers(ServersQuery{GroupID: mine.Group.ID}); slices.Contains(ids(inMine), mine.IDs[1]) {
		t.Fatalf("a moved server left its group: %v", ids(inMine))
	}
	if inSub, _ := e.Servers(ServersQuery{GroupID: sub.Group.ID}); !slices.Contains(ids(inSub), subServers[1].ID) {
		t.Fatalf("a server of a subscription stays there: %v", ids(inSub))
	}
	if _, err = e.MoveServers(MoveServers{IDs: []string{created.ID}, GroupID: sub.Group.ID}); reason(err) != ReasonNotManual {
		t.Fatalf("moving into a subscription: %v", err)
	}
	if groups, _ := e.Groups(); groups[1].Servers != 3 || groups[2].Servers != 2 {
		t.Fatalf("the groups count their servers: %+v", groups)
	}
}

func TestRemovedServerPassesTheChoiceOn(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	p := newProvider(t)
	p.serve(200, vless("de.example.com", "Germany"))
	e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	mine, _ := e.AddServers(AddServers{NewGroup: "Mine", Text: vless("fi.example.com", "Finland") + "\n" + vless("se.example.com", "Sweden")})

	settings, _ := e.Settings()
	settings.Server = mine.IDs[0]
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := e.RemoveServers(ServerIDs{IDs: []string{mine.IDs[0], "missing"}}); err != nil {
		t.Fatal(err)
	}
	if settings, _ = e.Settings(); settings.Server != mine.IDs[1] {
		t.Fatalf("the choice stays in the group: %q, want %q", settings.Server, mine.IDs[1])
	}
	if groups, _ := e.Groups(); groups[1].Servers != 1 {
		t.Fatalf("the group counts what is left: %+v", groups[1])
	}
}
