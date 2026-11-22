package engine

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"PorosCore/config"
	"PorosCore/internal/boxmain"
)

// The list of connections names the program of a rule as its file is, never the pattern the config is built of.
func TestRuleTextNamesPrograms(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	typedName, typedPath := strings.ToUpper(filepath.Base(exe)), strings.ToUpper(exe)
	for _, c := range []struct {
		what string
		rule config.Rule
		// shown gets the name and the path of the program as Windows tells them.
		shown func(name, path string) string
	}{
		{
			"a name", config.Rule{ProcessName: []string{typedName}},
			func(name, _ string) string { return "process_name=" + name },
		},
		{
			"one of the names", config.Rule{ProcessName: []string{"other app (x64).exe", typedName}},
			func(name, _ string) string { return "process_name=" + name },
		},
		{
			"a path", config.Rule{ProcessPath: []string{typedPath}},
			func(_, path string) string { return "process_path=" + path },
		},
		{
			"a name and a path", config.Rule{Network: []string{"tcp"}, ProcessName: []string{typedName}, ProcessPath: []string{typedPath}},
			func(name, path string) string {
				return "network=tcp && process_name=" + name + " && process_path=" + path
			},
		},
		{
			// The user's own pattern shows the program it found.
			"a pattern", config.Rule{ProcessPathRegex: []string{`(?i)` + regexp.QuoteMeta(typedName) + `$`}},
			func(name, _ string) string { return "process_name=" + name },
		},
		{
			"all but a name", config.Rule{Invert: true, ProcessName: []string{"Other.exe"}},
			func(_, _ string) string { return "!(process_name=Other.exe)" },
		},
	} {
		t.Run(c.what, func(t *testing.T) {
			e, states := openConnectionTest(t)
			page := webPage(t)
			port := freePort(t)
			server, cancel, err := boxmain.Create([]byte(fmt.Sprintf(`{"log":{"level":"warn"},`+
				`"inbounds":[{"type":"trojan","listen":"127.0.0.1","listen_port":%d,"users":[{"password":"secret"}]}],`+
				`"outbounds":[{"type":"direct","tag":"direct"}]}`, port)), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { cancel(); server.Close() })
			useServer(t, e, fmt.Sprintf("trojan://secret@127.0.0.1:%d?security=none#local", port))
			rule := c.rule
			rule.Action, rule.Outbound = config.ActionRoute, config.OutboundProxy
			e.presets = append(e.presets, presetRecord{ID: "programs", Preset: config.Preset{Name: "Programs", DefaultOutbound: config.OutboundBlock, Rules: []config.Rule{rule}}})
			settings, err := e.Settings()
			if err != nil {
				t.Fatal(err)
			}
			settings.Preset = "programs"
			if _, err = e.SetSettings(settings); err != nil {
				t.Fatal(err)
			}
			connectAndFetch(t, e, states, page)

			target, err := url.Parse(page)
			if err != nil {
				t.Fatal(err)
			}
			conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(e.mixedPort))))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target.Host, target.Host)
			if response, err := http.ReadResponse(bufio.NewReader(conn), nil); err != nil || response.StatusCode != http.StatusOK {
				t.Fatalf("CONNECT: %v, %v", response, err)
			}
			var found Connection
			for deadline := time.Now().Add(5 * time.Second); found.ID == "" && time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
				list, err := e.Connections()
				if err != nil {
					t.Fatal(err)
				}
				for _, listed := range list.Connections {
					if listed.Source == conn.LocalAddr().String() {
						found = listed
					}
				}
			}
			if found.ID == "" {
				t.Fatal("the connection is not listed")
			}
			if want := c.shown(found.Process, found.ProcessPath); found.Rule != want {
				t.Errorf("the list shows %q, want %q", found.Rule, want)
			}
		})
	}
}
