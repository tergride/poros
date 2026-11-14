package engine

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// The device data of the settings replace Poros's own, and a group's own replace them in turn; the debug
// information hides what may be private.
func TestDeviceDataAndDebugInfo(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	settings, _ := e.Settings()
	for _, wrong := range []string{"hwid", "hwid=", "serial=1", "os=Win\ndows"} {
		settings.DeviceData = wrong
		if _, err := e.SetSettings(settings); reason(err) != ReasonInvalidValue {
			t.Errorf("%q: %v", wrong, err)
		}
	}
	settings.DeviceData = " HWID=abc , os=Android, osVersion=14, model=Pixel 8 "
	settings.ProxyPassword, settings.DNSRemote = "s3cret", "https://dns.example.com/my-id"
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	own, _ := parseDeviceData(settings.DeviceData)
	headers := e.deviceHeadersOf(GroupSettings{OS: "Linux"}, own)
	if headers.Get("X-Hwid") != "abc" || headers.Get("X-Device-Os") != "Linux" || headers.Get("X-Ver-Os") != "14" || headers.Get("X-Device-Model") != "Pixel 8" {
		t.Errorf("the device of the settings under the group's: %v", headers)
	}
	text, err := e.DebugInfo()
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"s3cret", "my-id", "Pixel", "abc"} {
		if strings.Contains(text, private) {
			t.Errorf("the debug information shows %q:\n%s", private, text)
		}
	}
	for _, shown := range []string{"Poros 0.1.0", "sing-box", " goroutines, heap ", `"proxyPassword": "(set)"`, `"proxyUsername": ""`, `"tunMtu": 1500`} {
		if !strings.Contains(text, shown) {
			t.Errorf("the debug information lacks %q:\n%s", shown, text)
		}
	}
}
