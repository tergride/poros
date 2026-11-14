package engine

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const testUUID = "b831381d-6324-4d53-ad4f-8cda48b30811"

// provider is a subscription server on loopback.
type provider struct {
	mu      sync.Mutex
	status  int
	body    string
	headers map[string]string
	agents  []string
	hwids   []string
	url     string
}

func newProvider(t *testing.T) *provider {
	t.Helper()
	p := &provider{status: http.StatusOK, headers: map[string]string{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.agents = append(p.agents, r.UserAgent())
		p.hwids = append(p.hwids, r.Header.Get("X-Hwid"))
		for name, value := range p.headers {
			w.Header().Set(name, value)
		}
		w.WriteHeader(p.status)
		io.WriteString(w, p.body)
	}))
	t.Cleanup(server.Close)
	// The token stands for the secret a real subscription link carries.
	p.url = server.URL + "/u/secret-token"
	return p
}

func (p *provider) serve(status int, lines ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status, p.body = status, strings.Join(lines, "\n")
}

func (p *provider) requests() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.agents)
}

func vless(host, name string) string {
	return vlessPath(host, name, "/ws")
}

func vlessPath(host, name, path string) string {
	return "vless://" + testUUID + "@" + host + ":443?security=tls&type=ws&path=" + url.QueryEscape(path) + "#" + url.PathEscape(name)
}

func TestAddSubscriptionRejectsLinks(t *testing.T) {
	e, events := openTest(t, t.TempDir())
	for link, want := range map[string]string{
		"":                    ReasonInvalidURL,
		"sub.example.com/u/1": ReasonInvalidURL,
		"https://":            ReasonInvalidURL,
		"ftp://sub.example":   ReasonInvalidURL,
		// The simple mode adds no server by hand and tells the user why.
		vless("de.example.com", "DE"): ReasonSingleServer,
		"trojan://@broken.example":    ReasonSingleServer,
	} {
		if _, err := e.AddSubscription(t.Context(), AddSubscription{URL: link}); reason(err) != want {
			t.Errorf("%q: expected %s, got %v", link, want, err)
		}
	}
	if len(*events) != 0 {
		t.Fatalf("nothing was added, so nothing is announced: %+v", *events)
	}
}

func TestAddSubscription(t *testing.T) {
	e, events := openTest(t, t.TempDir())
	p := newProvider(t)
	p.headers["Profile-Title"] = "base64:" + base64.StdEncoding.EncodeToString([]byte("Основная подписка"))
	p.headers["Subscription-Userinfo"] = "upload=100; download=200; total=1000; expire=1765497600"
	p.serve(http.StatusOK,
		vless("de.example.com", "🇩🇪 Germany"),
		vless("nl.example.com", "🇳🇱 Netherlands"),
		"juicity://key@198.51.100.7:443#Juicity",
		"juicity://key@198.51.100.8:443#Juicity2",
		"trojan://@t.example.com#Broken",
	)

	update, err := e.AddSubscription(t.Context(), AddSubscription{URL: " " + p.url + " "})
	if err != nil {
		t.Fatalf("AddSubscription: %v", err)
	}
	sub := update.Group
	if sub.Name != "Основная подписка" || sub.URL != p.url || sub.Servers != 2 || sub.Skipped != 3 || sub.UpdatedAt == 0 ||
		sub.Upload != 100 || sub.Download != 200 || sub.Total != 1000 || sub.Expire != 1765497600000 || sub.Error != "" {
		t.Fatalf("subscription %+v", sub)
	}
	if update.Added != 2 || update.Removed != 0 || update.Changed != 0 ||
		!reflect.DeepEqual(update.AddedNames, []string{"🇩🇪 Germany", "🇳🇱 Netherlands"}) {
		t.Fatalf("changes %+v", update)
	}
	wantSkips := []SkipReason{
		{Reason: "juicity:// links are not supported", Unsupported: true, Count: 2, Names: []string{"Juicity", "Juicity2"}},
		{Reason: "the Trojan password is missing", Count: 1, Names: []string{"Broken"}},
	}
	if !reflect.DeepEqual(update.Skips, wantSkips) {
		t.Fatalf("skips\n got %+v\nwant %+v", update.Skips, wantSkips)
	}
	if !reflect.DeepEqual(p.agents, []string{"Poros/0.1.0"}) {
		t.Fatalf("user agents %q", p.agents)
	}
	if len(*events) != 2 || (*events)[0].method != EventGroupsChanged || (*events)[1].params != (ServersChanged{sub.ID}) {
		t.Fatalf("adding must announce the subscriptions and the servers: %+v", *events)
	}

	servers, _ := e.Servers(ServersQuery{})
	want := []Server{
		{GroupID: sub.ID, Name: "🇩🇪 Germany", Protocol: "vless", Host: "de.example.com", Port: 443, Country: "DE"},
		{GroupID: sub.ID, Name: "🇳🇱 Netherlands", Protocol: "vless", Host: "nl.example.com", Port: 443, Country: "NL"},
	}
	for i := range servers {
		if servers[i].ID == "" {
			t.Fatalf("server %d has no ID", i)
		}
		want[i].ID = servers[i].ID
	}
	if !reflect.DeepEqual(servers, want) {
		t.Fatalf("servers\n got %+v\nwant %+v", servers, want)
	}

	if _, err = e.AddSubscription(t.Context(), AddSubscription{URL: p.url}); reason(err) != ReasonDuplicate || p.requests() != 1 {
		t.Fatalf("the same link twice is refused before downloading: %v, %d requests", err, p.requests())
	}
	other := newProvider(t)
	other.serve(http.StatusOK, vless("fi.example.com", ""))
	named, err := e.AddSubscription(t.Context(), AddSubscription{URL: other.url, Name: " Work "})
	if err != nil || named.Group.Name != "Work" {
		t.Fatalf("the user's name wins over the host: %+v, %v", named.Group, err)
	}
	if servers, _ = e.Servers(ServersQuery{GroupID: named.Group.ID}); len(servers) != 1 || servers[0].Name != "fi.example.com:443" {
		t.Fatalf("a server without a name is named by its address: %+v", servers)
	}
	list, _ := e.Groups()
	if len(list) != 2 || list[0].ID != sub.ID || list[1].ID != named.Group.ID {
		t.Fatalf("subscriptions keep the order they were added in: %+v", list)
	}
}

func TestAddSubscriptionFailures(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	unreachable := gone.URL + "/u/secret-token"
	gone.Close()

	for name, tt := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"error page":       {http.StatusNotFound, "gone", ReasonFetchFailed},
		"empty":            {http.StatusOK, " \n", ReasonNoServers},
		"only unsupported": {http.StatusOK, "juicity://key@198.51.100.7:443#Juicity", ReasonNoServers},
		"no format":        {http.StatusOK, `{"log": {}}`, ReasonInvalidSubscription},
		"unreachable":      {0, "", ReasonFetchFailed},
	} {
		t.Run(name, func(t *testing.T) {
			e, events := openTest(t, t.TempDir())
			link := unreachable
			if tt.status != 0 {
				p := newProvider(t)
				p.serve(tt.status, tt.body)
				link = p.url
			}
			_, err := e.AddSubscription(t.Context(), AddSubscription{URL: link})
			if reason(err) != tt.want {
				t.Fatalf("expected %s, got %v", tt.want, err)
			}
			if strings.Contains(err.Error(), "secret-token") {
				t.Errorf("the error %q gives away the subscription's token", err)
			}
			if list, _ := e.Groups(); len(list) != 0 || len(*events) != 0 {
				t.Fatalf("a subscription that failed is not added: %+v, %+v", list, *events)
			}
		})
	}
}

func TestUpdateSubscriptionKeepsServerIDs(t *testing.T) {
	e, events := openTest(t, t.TempDir())
	p := newProvider(t)
	p.serve(http.StatusOK, vless("de.example.com", "DE"), vless("nl.example.com", "NL"), vless("fi.example.com", "FI"))
	added, err := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})
	if err != nil {
		t.Fatalf("AddSubscription: %v", err)
	}
	before, _ := e.Servers(ServersQuery{})

	// DE stays as it was, NL gets a new path, FI goes and SE comes.
	p.serve(http.StatusOK, vless("de.example.com", "DE"), vlessPath("nl.example.com", "NL", "/new"), vless("se.example.com", "SE"))
	*events = nil
	update, err := e.UpdateSubscription(t.Context(), added.Group.ID)
	if err != nil {
		t.Fatalf("UpdateSubscription: %v", err)
	}
	if update.Added != 1 || update.Removed != 1 || update.Changed != 1 ||
		!reflect.DeepEqual(update.AddedNames, []string{"SE"}) || !reflect.DeepEqual(update.RemovedNames, []string{"FI"}) {
		t.Fatalf("changes %+v", update)
	}
	after, _ := e.Servers(ServersQuery{})
	if len(after) != 3 || after[0].ID != before[0].ID || after[1].ID != before[1].ID || after[2].Name != "SE" ||
		after[2].ID == before[2].ID {
		t.Fatalf("servers that continue keep their IDs\nbefore %+v\n after %+v", before, after)
	}
	if len(*events) != 2 || (*events)[1].params != (ServersChanged{added.Group.ID}) {
		t.Fatalf("an update announces the subscriptions and the servers: %+v", *events)
	}
	if _, err = e.UpdateSubscription(t.Context(), "missing"); reason(err) != ReasonNotFound {
		t.Fatalf("updating an unknown id: %v", err)
	}
}

func TestFailedUpdateKeepsServers(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	clock := time.UnixMilli(1_000_000)
	e.now = func() time.Time { return clock }
	p := newProvider(t)
	p.serve(http.StatusOK, vless("de.example.com", "DE"))
	added, _ := e.AddSubscription(t.Context(), AddSubscription{URL: p.url})

	clock = clock.Add(time.Hour)
	p.serve(http.StatusInternalServerError, "down")
	if _, err := e.UpdateSubscription(t.Context(), added.Group.ID); reason(err) != ReasonFetchFailed {
		t.Fatalf("expected %s, got %v", ReasonFetchFailed, err)
	}
	list, _ := e.Groups()
	if sub := list[0]; sub.Error != ReasonFetchFailed || !strings.Contains(sub.ErrorText, "500") || sub.Servers != 1 ||
		sub.UpdatedAt != added.Group.UpdatedAt {
		t.Fatalf("a failed update keeps the servers and says why: %+v", sub)
	}
	if servers, _ := e.Servers(ServersQuery{}); len(servers) != 1 {
		t.Fatalf("servers after a failed update: %+v", servers)
	}

	p.serve(http.StatusOK, vless("de.example.com", "DE"))
	update, err := e.UpdateSubscription(t.Context(), added.Group.ID)
	if err != nil || update.Group.Error != "" || update.Group.ErrorText != "" {
		t.Fatalf("an update that works clears the error: %+v, %v", update.Group, err)
	}
}

func TestRemoveGroupAndRestart(t *testing.T) {
	dir := t.TempDir()
	e, _ := openTest(t, dir)
	first, second := newProvider(t), newProvider(t)
	first.serve(http.StatusOK, vless("de.example.com", "DE"))
	second.serve(http.StatusOK, vless("nl.example.com", "NL"))
	a, _ := e.AddSubscription(t.Context(), AddSubscription{URL: first.url})
	b, _ := e.AddSubscription(t.Context(), AddSubscription{URL: second.url})

	if err := e.RemoveGroup("missing"); reason(err) != ReasonNotFound {
		t.Fatalf("removing an unknown id: %v", err)
	}
	if err := e.RemoveGroup(a.Group.ID); err != nil {
		t.Fatalf("RemoveGroup: %v", err)
	}
	kept, _ := e.Servers(ServersQuery{})

	e.Close()
	reopened, _ := openTest(t, dir)
	list, _ := reopened.Groups()
	servers, _ := reopened.Servers(ServersQuery{})
	if len(list) != 1 || list[0].ID != b.Group.ID || !reflect.DeepEqual(servers, kept) || len(servers) != 1 {
		t.Fatalf("after a restart\nsubscriptions %+v\nservers %+v", list, servers)
	}
}

func TestDueAt(t *testing.T) {
	updated := time.UnixMilli(10_000_000)
	defaults := DefaultSettings()
	every24 := defaults
	every24.SubscriptionUpdateHours = 24
	for name, tt := range map[string]struct {
		rec      groupRecord
		settings Settings
		want     time.Duration
	}{
		"no advice":           {groupRecord{UpdatedAt: updated.UnixMilli()}, defaults, 6 * time.Hour},
		"provider's advice":   {withInterval(updated, 12*time.Hour), defaults, 12 * time.Hour},
		"the user's interval": {withInterval(updated, 12*time.Hour), every24, 24 * time.Hour},
		"retry after failure": {groupRecord{UpdatedAt: updated.UnixMilli(), CheckedAt: updated.Add(time.Minute).UnixMilli()}, defaults, time.Hour + time.Minute},
	} {
		if got := dueAt(tt.rec, tt.settings).Sub(updated); got != tt.want {
			t.Errorf("%s: due after %v, want %v", name, got, tt.want)
		}
	}
}

func withInterval(updated time.Time, interval time.Duration) groupRecord {
	rec := groupRecord{UpdatedAt: updated.UnixMilli()}
	rec.Info.UpdateInterval = interval
	return rec
}

func TestUpdateDue(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	clock := time.UnixMilli(1_000_000)
	e.now = func() time.Time { return clock }
	p := newProvider(t)
	p.serve(http.StatusOK, vless("de.example.com", "DE"))
	if _, err := e.AddSubscription(t.Context(), AddSubscription{URL: p.url}); err != nil {
		t.Fatalf("AddSubscription: %v", err)
	}

	clock = clock.Add(5 * time.Hour)
	e.updateDue(t.Context())
	if p.requests() != 1 {
		t.Fatalf("not due yet, but fetched %d times", p.requests())
	}
	clock = clock.Add(2 * time.Hour)
	settings := DefaultSettings()
	settings.SubscriptionAutoUpdate = false
	e.SetSettings(settings)
	e.updateDue(t.Context())
	if p.requests() != 1 {
		t.Fatal("automatic updates are off, but the subscription was fetched")
	}
	settings.SubscriptionAutoUpdate = true
	e.SetSettings(settings)
	e.updateDue(t.Context())
	if p.requests() != 2 {
		t.Fatalf("due and on, fetched %d times", p.requests())
	}
}

func TestFlagCountry(t *testing.T) {
	for name, want := range map[string]string{"🇩🇪 Germany": "DE", "Fast 🇺🇸 NY": "US", "Germany": "", "🏴 Wales": "", "": ""} {
		if got := flagCountry(name); got != want {
			t.Errorf("%q: got %q, want %q", name, got, want)
		}
	}
}
