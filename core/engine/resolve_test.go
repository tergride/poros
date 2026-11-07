package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestLookupPastSkipsFakeAddresses(t *testing.T) {
	answers := map[string]string{"/fake": "198.18.0.9", "/real": "192.0.2.7"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "1" {
			fmt.Fprint(w, `{"Answer":[]}`)
			return
		}
		fmt.Fprintf(w, `{"Answer":[{"type":5,"data":"cname.example."},{"type":1,"data":%q}]}`, answers[r.URL.Path])
	}))
	defer server.Close()
	servers, client := dohServers, dohClient
	t.Cleanup(func() { dohServers, dohClient = servers, client })
	dohClient = server.Client
	dohServers = []string{server.URL + "/fake", server.URL + "/real"}
	ctx := context.Background()

	// The system cannot resolve the name, or answers it with a fake address next to Throne: DoH answers, and a fake
	// answer there is passed over too.
	if addr, err := lookupPast(ctx, "poros-test.invalid", true); err != nil || addr != netip.MustParseAddr("192.0.2.7") {
		t.Fatalf("got %v, %v", addr, err)
	}
	if addr, err := lookupPast(ctx, "203.0.113.5", true); err != nil || addr != netip.MustParseAddr("203.0.113.5") {
		t.Fatalf("an address is its own answer: %v, %v", addr, err)
	}
	dohServers = []string{server.URL + "/fake"}
	if addr, err := lookupPast(ctx, "poros-test.invalid", true); err == nil {
		t.Fatalf("only fake answers must fail, got %v", addr)
	}
}
