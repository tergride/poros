package engine

import (
	"crypto/tls"
	"net/http"
	"reflect"
	"testing"
	"time"

	"PorosCore/profile"
)

func trojanLink(host, name string) string {
	return "trojan://secret@" + host + ":443?security=tls&type=ws&path=%2Fws#" + name
}

func TestConfigureGroup(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	p := newProvider(t)
	p.serve(http.StatusOK, trojanLink("a.example.com", "A"), trojanLink("b.example.com", "B"))
	sub, err := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	if err != nil {
		t.Fatal(err)
	}
	mine, err := e.AddServers(AddServers{NewGroup: "Mine", Text: trojanLink("front.example.com", "Front") + "\n" + trojanLink("land.example.com", "Landing")})
	if err != nil {
		t.Fatal(err)
	}
	front, landing := mine.IDs[0], mine.IDs[1]
	subServers, _ := e.Servers(ServersQuery{GroupID: sub.Group.ID})

	for name, tt := range map[string]struct {
		id   string
		gs   GroupSettings
		want string
	}{
		"a bad address":           {sub.Group.ID, GroupSettings{ServerAddress: "not a host"}, ReasonInvalidValue},
		"a proxy gone":            {sub.Group.ID, GroupSettings{FrontProxy: "missing"}, ReasonNotFound},
		"a proxy of the group":    {sub.Group.ID, GroupSettings{FrontProxy: subServers[0].ID}, ReasonInvalidValue},
		"the same proxy twice":    {sub.Group.ID, GroupSettings{FrontProxy: front, LandingProxy: front}, ReasonInvalidValue},
		"hours without a number":  {sub.Group.ID, GroupSettings{Update: GroupUpdateHours}, ReasonInvalidValue},
		"sorting without a test":  {sub.Group.ID, GroupSettings{SortByLatency: true}, ReasonInvalidValue},
		"updates of a user group": {mine.Group.ID, GroupSettings{Update: GroupUpdateNever}, ReasonInvalidValue},
		"an unknown group":        {"missing", GroupSettings{}, ReasonNotFound},
	} {
		if _, err := e.ConfigureGroup(ConfigureGroup{ID: tt.id, Settings: tt.gs}); reason(err) != tt.want {
			t.Errorf("%s: %v, want %s", name, err, tt.want)
		}
	}

	settings := GroupSettings{ServerAddress: "104.16.1.1", FrontProxy: front, LandingProxy: landing, Update: GroupUpdateHours, UpdateHours: 12}
	group, err := e.ConfigureGroup(ConfigureGroup{ID: sub.Group.ID, Settings: settings})
	if err != nil || group.Settings != settings {
		t.Fatalf("ConfigureGroup: %+v, %v", group, err)
	}
	if _, err = e.ConfigureGroup(ConfigureGroup{ID: mine.Group.ID, URL: "https://x.example.com/s"}); reason(err) != ReasonInvalidValue {
		t.Errorf("a URL for a group of the user's: %v", err)
	}

	// The server goes through the front proxy at the CDN address, keeping its name, and leaves by the landing proxy.
	p0, links, found, err := e.serverWithLinks(subServers[0].ID)
	if err != nil || !found || p0.Type != profile.TypeChain || len(links) != 3 {
		t.Fatalf("a server of the group: %+v, %d links, %v", p0, len(links), err)
	}
	if links[0].Server != "front.example.com" || links[2].Server != "land.example.com" {
		t.Errorf("the proxies around the server: %s, %s", links[0].Server, links[2].Server)
	}
	middle := links[1]
	if middle.Server != "104.16.1.1" || middle.Trojan.TLS.ServerName != "a.example.com" || middle.Trojan.Transport.Host != "a.example.com" {
		t.Errorf("the group's address keeps the server's name: %+v", middle.Trojan)
	}
	if err = e.RemoveServers(ServerIDs{IDs: []string{landing}}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = e.serverWithLinks(subServers[0].ID); err == nil {
		t.Error("a landing proxy gone must fail the server, not let it go around")
	}
}

func TestUpdateActions(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	p := newProvider(t)
	p.serve(http.StatusOK, trojanLink("a.example.com", "A"), trojanLink("b.example.com", "B"))
	sub, err := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := e.Servers(ServersQuery{GroupID: sub.Group.ID})
	_, err = e.ConfigureGroup(ConfigureGroup{ID: sub.Group.ID, Settings: GroupSettings{
		UserAgent: "clash-verge/v2", DeviceInfo: "on", DeviceID: "my-hwid", KeepWorking: true, RemoveDuplicates: true, RemoveInsecure: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	// B worked in the last test and is dropped by the provider; the duplicate of A and the plain one go.
	e.latency.mu.Lock()
	e.latency.results[before[1].ID] = 120
	e.latency.mu.Unlock()
	p.serve(http.StatusOK, trojanLink("a.example.com", "A"), trojanLink("a.example.com", "A again"),
		"trojan://secret@plain.example.com:80?security=none#Plain", trojanLink("c.example.com", "C"))
	if _, err = e.UpdateSubscription(t.Context(), sub.Group.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := e.Servers(ServersQuery{GroupID: sub.Group.ID})
	if got := names(after); !reflect.DeepEqual(got, []string{"A", "C", "B"}) {
		t.Errorf("after the update: %q", got)
	}
	p.mu.Lock()
	agent, hwid := p.agents[len(p.agents)-1], p.hwids[len(p.hwids)-1]
	p.mu.Unlock()
	if agent != "clash-verge/v2" || hwid != "my-hwid" {
		t.Errorf("the request of the group: User-Agent %q, HWID %q", agent, hwid)
	}

	// By a test's results: the server that did not answer goes, the fastest comes first.
	_, err = e.ConfigureGroup(ConfigureGroup{ID: sub.Group.ID, Settings: GroupSettings{Test: true, RemoveUnavailable: true, SortByLatency: true}})
	if err != nil {
		t.Fatal(err)
	}
	e.afterTest(sub.Group.ID, map[string]int{after[0].ID: 300, after[1].ID: -1, after[2].ID: 80})
	sorted, _ := e.Servers(ServersQuery{GroupID: sub.Group.ID})
	if got := names(sorted); !reflect.DeepEqual(got, []string{"B", "A"}) {
		t.Errorf("after the test: %q", got)
	}
}

func TestGroupSchedule(t *testing.T) {
	now := time.Now()
	rec := groupRecord{Kind: GroupSubscription, UpdatedAt: now.Add(-3 * time.Hour).UnixMilli()}
	off := Settings{SubscriptionAutoUpdate: false}
	if updatesItself(rec, off) {
		t.Error("a group as the settings say updates only with the updates on")
	}
	rec.Settings = GroupSettings{Update: GroupUpdateHours, UpdateHours: 2}
	if !updatesItself(rec, off) || !dueAt(rec, off).Before(now) {
		t.Error("a group of its own interval updates whatever the settings say, when its time comes")
	}
	rec.Settings = GroupSettings{Update: GroupUpdateNever}
	if updatesItself(rec, Settings{SubscriptionAutoUpdate: true}) {
		t.Error("a group that never updates by itself")
	}
}

func TestProfileSecurity(t *testing.T) {
	for link, want := range map[string]string{
		trojanLink("t.example.com", "TLS"):                           profile.SecurityOK,
		"trojan://pw@t.example.com:443?security=tls&allowInsecure=1": profile.SecurityWeak,
		"trojan://pw@t.example.com:80?security=none":                 profile.SecurityNone,
		"trojan://pw@192.168.1.10:80?security=none":                  profile.SecurityOK,
		"socks5://u:p@s.example.com:1080":                            profile.SecurityNone,
		"ss://cmM0LW1kNTpwdw@ss.example.com:8388":                    profile.SecurityWeak,
	} {
		p, err := profile.ParseLink(link)
		if err != nil {
			t.Fatalf("%s: %v", link, err)
		}
		if got := p.Security(); got != want {
			t.Errorf("%s: %q, want %q", link, got, want)
		}
	}
}

func TestSubscriptionRequestVersions(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	rec := groupRecord{Kind: GroupSubscription, Settings: GroupSettings{TLS: "1.3", HTTP: "1.1"}}
	request := e.requestOf(rec, Settings{SubscriptionTLS: "1.2", SubscriptionUserAgent: "v2rayN"})
	client := request.client(e.client)
	transport := client.Transport.(*http.Transport)
	if request.userAgent != "v2rayN" || transport.TLSClientConfig.MinVersion != tls.VersionTLS13 || transport.ForceAttemptHTTP2 ||
		transport.TLSNextProto == nil || client == e.client {
		t.Errorf("the group's TLS 1.3 and HTTP/1.1 over the settings: %+v", request)
	}
	if plain := e.requestOf(groupRecord{Settings: GroupSettings{TLS: "auto"}}, Settings{SubscriptionTLS: "1.2"}); plain.client(e.client) != e.client {
		t.Error("auto takes any TLS version with Poros's own client")
	}
}
