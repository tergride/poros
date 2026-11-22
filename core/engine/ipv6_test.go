package engine

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"PorosCore/config"
	"PorosCore/internal/boxmain"
)

func TestIPv6IsOffByDefault(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	settings, err := e.Settings()
	if err != nil || settings.IPv6 {
		t.Fatalf("new data: IPv6 %t, %v", settings.IPv6, err)
	}
	// What was saved stays.
	settings.IPv6 = true
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if saved, err := e.Settings(); err != nil || !saved.IPv6 {
		t.Errorf("saved as on: IPv6 %t, %v", saved.IPv6, err)
	}
}

// Without IPv6 Poros resolves a name before the rules: those on names and on programs still take the connection.
func TestRulesMatchAfterResolvingToIPv4(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for what, rule := range map[string]config.Rule{
		"a name":    {DomainSuffix: []string{"site.example"}},
		"a program": {ProcessName: []string{filepath.Base(exe)}},
	} {
		t.Run(what, func(t *testing.T) {
			e, states := openConnectionTest(t)
			page, err := url.Parse(webPage(t))
			if err != nil {
				t.Fatal(err)
			}
			port := freePort(t)
			server, cancel, err := boxmain.Create([]byte(fmt.Sprintf(`{"log":{"level":"warn"},`+
				`"inbounds":[{"type":"trojan","listen":"127.0.0.1","listen_port":%d,"users":[{"password":"secret"}]}],`+
				`"outbounds":[{"type":"direct","tag":"direct"}]}`, port)), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { cancel(); server.Close() })
			useServer(t, e, fmt.Sprintf("trojan://secret@127.0.0.1:%d?security=none#local", port))

			rule.Action, rule.Outbound = config.ActionRoute, config.OutboundProxy
			e.presets = append(e.presets, presetRecord{ID: "ruled", Preset: config.Preset{Name: "Ruled", DefaultOutbound: config.OutboundBlock, Rules: []config.Rule{rule}}})
			settings, err := e.Settings()
			if err != nil {
				t.Fatal(err)
			}
			settings.IPv6, settings.Preset, settings.DNSAnswers = false, "ruled", "site.example 127.0.0.1"
			if _, err = e.SetSettings(settings); err != nil {
				t.Fatal(err)
			}
			connectAndFetch(t, e, states, "http://site.example:"+page.Port())
		})
	}
}
