package netcheck

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// The answers are shaped like the real ones of 2026-11-05, with documentation addresses and AS numbers.
const (
	ipLeakAnswer = `{"as_number":64500,"isp_name":"Example Networks","as_network":"203.0.113.0/24","country_code":"DE",` +
		`"country_name":"Germany","region_name":"Hesse","city_name":"Frankfurt am Main","latitude":50.1,"longitude":8.6,` +
		`"ip":"203.0.113.10","reverse":"","query_text":"203.0.113.10","query_type":"myip"}`
	ipInfoAnswer = `{"ip":"203.0.113.11","city":"Amsterdam","region":"North Holland","country":"NL","loc":"52.37,4.89",` +
		`"org":"AS64501 Example Hosting B.V.","postal":"1012","timezone":"Europe/Amsterdam"}`
	ipWhoAnswer = `{"ip":"203.0.113.12","success":true,"type":"IPv4","country":"Finland","country_code":"FI","city":"Helsinki",` +
		`"connection":{"asn":64502,"org":"Example Oy","isp":"Example Telecom","domain":"example.fi"}}`
	leakAnswer = `[{"ip":"198.51.100.20","country":"de","country_name":"Germany","asn":"AS64500 Example Networks","org":"Example Networks",` +
		`"region_name":"Hesse","city_name":"Frankfurt am Main","type":"ip"},` +
		`{"ip":"192.0.2.53","country":"us","country_name":"United States","asn":"AS64510 Example DNS","type":"dns"},` +
		`{"ip":"192.0.2.54","country":"us","country_name":"United States","asn":"AS64510 Example DNS","type":"dns"},` +
		`{"ip":"","type":"conclusion"}]`
)

func TestParsers(t *testing.T) {
	cases := []struct {
		name  string
		parse func([]byte) (Info, error)
		body  string
		want  Info
	}{
		{"ipleak", parseIPLeak, ipLeakAnswer, Info{"203.0.113.10", "DE", "Frankfurt am Main", "Example Networks", 64500}},
		{"ipinfo", parseIPInfo, ipInfoAnswer, Info{"203.0.113.11", "NL", "Amsterdam", "Example Hosting B.V.", 64501}},
		{"ipwho", parseIPWho, ipWhoAnswer, Info{"203.0.113.12", "FI", "Helsinki", "Example Telecom", 64502}},
	}
	for _, c := range cases {
		got, err := c.parse([]byte(c.body))
		if err != nil || got != c.want {
			t.Errorf("%s: %+v, %v", c.name, got, err)
		}
	}
	if _, err := parseIPWho([]byte(`{"success":false,"message":"limit reached"}`)); err == nil {
		t.Error("ipwho.is refusing must be an error, so the next service is asked")
	}
}

func serve(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path]
		if !ok {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestLookupFallsBackToTheNextService(t *testing.T) {
	server := serve(t, map[string]string{"/b": ipInfoAnswer})
	services := []Service{
		{URL: server.URL + "/a", Parse: parseIPLeak},
		{URL: server.URL + "/b", Parse: parseIPInfo},
	}
	info, err := Lookup(context.Background(), server.Client(), services)
	if err != nil || info.IP != "203.0.113.11" {
		t.Fatalf("Lookup: %+v, %v", info, err)
	}
	if _, err = Lookup(context.Background(), server.Client(), services[:1]); err == nil {
		t.Fatal("with every service down Lookup must fail")
	}
}

func TestLeakProbesUnderTheTestID(t *testing.T) {
	server := serve(t, map[string]string{"/id": "00d1e2f3a4b5c6d7\n", "/dnsleak/test/00d1e2f3a4b5c6d7": leakAnswer})
	var mu sync.Mutex
	var asked []string
	resolve := func(_ context.Context, host string) error {
		mu.Lock()
		defer mu.Unlock()
		asked = append(asked, host)
		return nil
	}
	servers, err := Leak(context.Background(), server.Client(), server.URL, resolve)
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != probes || !strings.Contains(asked[0], ".00d1e2f3a4b5c6d7.127.0.0.1:") {
		t.Errorf("probes: %v", asked)
	}
	if len(servers) != 2 || servers[0] != (Info{"192.0.2.53", "US", "", "Example DNS", 64510}) {
		t.Errorf("servers: %+v", servers)
	}

	failing := serve(t, map[string]string{"/id": "abc", "/dnsleak/test/abc": `[{"type":"error","text":"No DNS servers found."}]`})
	if _, err = Leak(context.Background(), failing.Client(), failing.URL, resolve); err == nil {
		t.Error("an error record must fail the test")
	}
}
