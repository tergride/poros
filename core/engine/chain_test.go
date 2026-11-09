package engine

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"PorosCore/profile"
)

func chainProfile(ids ...string) profile.Profile {
	return profile.Profile{Type: profile.TypeChain, Name: "Chain", Chain: &profile.Chain{Servers: ids}}
}

func TestSaveCompoundServers(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	mine, err := e.AddServers(AddServers{NewGroup: "Mine", Text: vless("fi.example.com", "Finland") + "\n" + vless("se.example.com", "Sweden")})
	if err != nil {
		t.Fatal(err)
	}
	fi, se := mine.IDs[0], mine.IDs[1]
	chain, err := e.SaveServer(SaveServer{GroupID: mine.Group.ID, Profile: chainProfile(fi, se)})
	if err != nil || chain.Name != "Chain" || chain.Protocol != profile.TypeChain || chain.Host != "" {
		t.Fatalf("a chain: %+v, %v", chain, err)
	}
	direct, err := e.SaveServer(SaveServer{GroupID: mine.Group.ID, Profile: profile.Profile{Type: profile.TypeDirect, Direct: &profile.Direct{}}})
	if err != nil || direct.Name != profile.TypeDirect {
		t.Fatalf("Direct without a name is named by its type: %+v, %v", direct, err)
	}
	for name, tt := range map[string]struct {
		id   string
		p    profile.Profile
		want string
	}{
		"a server gone":   {"", chainProfile(fi, "missing"), ReasonNotFound},
		"through itself":  {chain.ID, chainProfile(fi, chain.ID), ReasonInvalidValue},
		"through a chain": {"", chainProfile(fi, chain.ID), ReasonInvalidValue},
		"through Direct":  {"", chainProfile(direct.ID, se), ReasonInvalidValue},
		"a custom sing-box refuses": {"", profile.Profile{Type: profile.TypeCustom, Custom: &profile.Custom{
			JSON: json.RawMessage(`{"type": "trojan", "server": "c.example.com", "server_port": 443, "password": "pw", "unknown_field": 1}`)}},
			ReasonInvalidValue},
	} {
		if _, err := e.SaveServer(SaveServer{ID: tt.id, GroupID: mine.Group.ID, Profile: tt.p}); reason(err) != tt.want {
			t.Errorf("%s: %v, want %s", name, err, tt.want)
		}
	}
	custom := profile.Profile{Type: profile.TypeCustom, Name: "Custom", Custom: &profile.Custom{
		JSON: json.RawMessage(`{"type": "trojan", "server": "c.example.com", "server_port": 443, "password": "pw"}`)}}
	if _, err := e.SaveServer(SaveServer{GroupID: mine.Group.ID, Profile: custom}); err != nil {
		t.Errorf("a custom outbound sing-box takes: %v", err)
	}
	if _, err := e.ShareLinks(ServerIDs{IDs: []string{chain.ID}}); reason(err) != ReasonInvalidValue {
		t.Errorf("a chain has no link: %v", err)
	}
	if out, err := e.ExportServer(ByID{ID: chain.ID}); err != nil || out.Core != CoreSingBox {
		t.Errorf("a chain is exported with its servers: %+v, %v", out, err)
	}
}

// A chain of two Trojan servers on loopback: the page opens only while both of them run.
func TestConnectionThroughAChain(t *testing.T) {
	e, states := openConnectionTest(t)
	page := webPage(t)
	first, second := freePort(t), freePort(t)
	for _, port := range []uint16{first, second} {
		runServer(t, fmt.Sprintf(`{"log":{"level":"warn"},"inbounds":[{"type":"trojan","listen":"127.0.0.1","listen_port":%d,`+
			`"users":[{"password":"secret"}]}],"outbounds":[{"type":"direct","tag":"direct"}]}`, port))
	}
	added, err := e.AddServers(AddServers{NewGroup: "Mine", Text: fmt.Sprintf(
		"trojan://secret@127.0.0.1:%d?security=none#first\ntrojan://secret@127.0.0.1:%d?security=none#second", first, second)})
	if err != nil {
		t.Fatal(err)
	}
	chain, err := e.SaveServer(SaveServer{GroupID: added.Group.ID, Profile: chainProfile(added.IDs...)})
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.Server, settings.Preset = chain.ID, "through"
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	connectAndFetch(t, e, states, page)
	if got, want := e.ConnectionState().Servers, append([]string{chain.ID}, added.IDs...); !slices.Equal(got, want) {
		t.Errorf("the connection goes through %v, want %v", got, want)
	}
	e.StopConnection()
	waitState(t, states)

	// A server of the chain gone: the start says so instead of going around it.
	if err = e.RemoveServers(ServerIDs{IDs: added.IDs[:1]}); err != nil {
		t.Fatal(err)
	}
	e.StartConnection()
	if state := waitState(t, states); state.State != ConnectionIdle || state.Error != ReasonNoServer {
		t.Fatalf("a chain with a server gone: %+v", state)
	}
}
