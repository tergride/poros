package engine

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"PorosCore/config"
)

// «Network» and «Security»: the clock of the core, what is refused, and what the certificate settings give a
// connection.
func TestNetworkSettings(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	settings, err := e.Settings()
	if err != nil || settings.NTP || settings.NTPServer != defaultNTPServer || settings.NTPPort != 123 || settings.NTPInterval != 30 ||
		settings.NTPOutbound != config.OutboundDirect || settings.IgnoreTLSErrors || settings.XraySystemCerts || settings.InsecureDefault {
		t.Fatalf("the defaults: %+v, %v", settings, err)
	}
	if ntpOf(settings) != nil {
		t.Error("the clock is the system's by default")
	}
	for name, change := range map[string]func(s *Settings){
		"an outbound":           func(s *Settings) { s.NTPOutbound = "warp" },
		"no server":             func(s *Settings) { s.NTP, s.NTPServer = true, " " },
		"an address with a URL": func(s *Settings) { s.NTPServer = "ntp://time.example.com" },
		"a port":                func(s *Settings) { s.NTPPort = 0 },
		"an interval":           func(s *Settings) { s.NTPInterval = 0 },
		"a long interval":       func(s *Settings) { s.NTPInterval = maxNTPInterval + 1 },
	} {
		wrong := settings
		change(&wrong)
		if _, err := e.SetSettings(wrong); reason(err) != ReasonInvalidValue {
			t.Errorf("%s: %v", name, err)
		}
	}
	settings.NTP, settings.NTPServer, settings.NTPPort, settings.NTPInterval, settings.NTPOutbound = true, " 2606:4700:f1::1 ", 1123, 15, config.OutboundProxy
	settings.XraySystemCerts, settings.InsecureDefault = true, true
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if ntp := ntpOf(settings); ntp == nil || ntp.Server != "2606:4700:f1::1" || ntp.Port != 1123 || ntp.Interval != 15*time.Minute || !ntp.Proxy {
		t.Errorf("the clock of a connection: %+v", ntp)
	}
	if tune := tuningOf(settings); !tune.Insecure || !tune.XraySystemCA {
		t.Errorf("the certificates of a connection: %+v", tune)
	}
}

// «Ignore TLS errors» lets subscriptions download from a server whose certificate does not check out.
func TestIgnoreTLSErrors(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, vless("v.example.com", "Self-signed"))
	}))
	t.Cleanup(server.Close)
	link := server.URL + "/u/secret-token"
	if _, err := e.AddSubscription(t.Context(), AddSubscription{URL: link}); reason(err) != ReasonFetchFailed {
		t.Fatalf("a certificate that does not check out: %v", err)
	}
	settings, _ := e.Settings()
	settings.IgnoreTLSErrors = true
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if update, err := e.AddSubscription(t.Context(), AddSubscription{URL: link}); err != nil || update.Added != 1 {
		t.Errorf("with TLS errors ignored: %+v, %v", update, err)
	}
	if plain := e.requestOf(groupRecord{}, Settings{}); plain.client(e.client) != e.client {
		t.Error("without the setting the request takes Poros's own client")
	}
}
