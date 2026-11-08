package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
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
	if addr, err := lookupPast(ctx, net.DefaultResolver, "poros-test.invalid", true); err != nil || addr != netip.MustParseAddr("192.0.2.7") {
		t.Fatalf("got %v, %v", addr, err)
	}
	if addr, err := lookupPast(ctx, net.DefaultResolver, "203.0.113.5", true); err != nil || addr != netip.MustParseAddr("203.0.113.5") {
		t.Fatalf("an address is its own answer: %v, %v", addr, err)
	}
	dohServers = []string{server.URL + "/fake"}
	if addr, err := lookupPast(ctx, net.DefaultResolver, "poros-test.invalid", true); err == nil {
		t.Fatalf("only fake answers must fail, got %v", addr)
	}
}

// Behind the kill switch the system's DNS may never answer: DoH must still have its time.
func TestLookupPastOutlastsAHangingSystem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"Answer":[{"type":1,"data":"192.0.2.8"}]}`)
	}))
	defer server.Close()
	servers, client, timeout := dohServers, dohClient, systemLookupTimeout
	t.Cleanup(func() { dohServers, dohClient, systemLookupTimeout = servers, client, timeout })
	dohClient = server.Client
	dohServers = []string{server.URL}
	systemLookupTimeout = 100 * time.Millisecond
	hanging := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if addr, err := lookupPast(ctx, hanging, "poros-test.invalid", true); err != nil || addr != netip.MustParseAddr("192.0.2.8") {
		t.Fatalf("got %v, %v", addr, err)
	}
}
