package engine

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"PorosCore/profile"
)

func TestCheckServer(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	mine, err := e.AddServers(AddServers{NewGroup: "Mine", Text: vless("fi.example.com", "Finland") + "\n" + vless("se.example.com", "Sweden")})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := e.Server(ByID{ID: mine.IDs[0]})
	if err != nil {
		t.Fatal(err)
	}
	ws := stored.Profile
	xhttp := ws
	xhttpPart := *ws.VLESS
	xhttpPart.Transport = profile.Transport{Type: profile.TransportXHTTP, Path: "/x"}
	xhttpPart.TLS = profile.TLS{Enabled: true, ServerName: "fi.example.com"}
	xhttp.VLESS = &xhttpPart
	noUUID := ws
	noUUIDPart := *ws.VLESS
	noUUIDPart.UUID = ""
	noUUID.VLESS = &noUUIDPart
	// ShadowTLS from «New profile»: its TLS is on by itself.
	shadowTLS := profile.Profile{Type: profile.TypeShadowsocks, Server: "stls.example.com", ServerPort: 443, Shadowsocks: &profile.Shadowsocks{
		Method: "aes-128-gcm", Password: "pw", ShadowTLS: profile.ShadowTLS{Version: 3, Password: "pw", TLS: profile.TLS{ServerName: "www.example.com"}}}}

	for name, tt := range map[string]struct {
		id      string
		p       profile.Profile
		core    string
		problem string
		link    bool
	}{
		"VLESS over WebSocket": {p: ws, core: CoreSingBox, link: true},
		"VLESS over XHTTP":     {p: xhttp, core: CoreXray, link: true},
		"ShadowTLS":            {p: shadowTLS, core: CoreSingBox, link: true},
		"no UUID":              {p: noUUID, core: CoreSingBox, problem: "the VLESS UUID is missing"},
		"a chain":              {p: chainProfile(mine.IDs...), core: CoreSingBox},
		"a chain through itself": {id: mine.IDs[0], p: chainProfile(mine.IDs...), core: CoreSingBox,
			problem: "a chain cannot go through itself"},
		"a custom sing-box refuses": {p: profile.Profile{Type: profile.TypeCustom, Custom: &profile.Custom{
			JSON: json.RawMessage(`{"type": "trojan", "server": "c.example.com", "server_port": 443, "password": "pw", "unknown": 1}`)}},
			core: CoreSingBox, problem: "unknown"},
	} {
		check, err := e.CheckServer(SaveServer{ID: tt.id, Profile: tt.p})
		switch {
		case err != nil:
			t.Errorf("%s: %v", name, err)
		case check.Core != tt.core:
			t.Errorf("%s runs on %s, want %s", name, check.Core, tt.core)
		case tt.problem == "" && check.Problem != "":
			t.Errorf("%s: %s", name, check.Problem)
		case !strings.Contains(check.Problem, tt.problem):
			t.Errorf("%s: problem %q, want %q", name, check.Problem, tt.problem)
		case tt.link != (check.Link != ""):
			t.Errorf("%s: link %q", name, check.Link)
		}
	}
}

// The stub core of the interface draws the editor from the same forms. POROS_UPDATE_FORMS=1 writes them anew.
func TestStubForms(t *testing.T) {
	data, err := json.MarshalIndent(ServerForms{Forms: profile.Forms(), New: profile.NewProfiles()}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := filepath.Join("..", "..", "src", "platform", "stubForms.json")
	if os.Getenv("POROS_UPDATE_FORMS") != "" {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
		t.Errorf("src/platform/stubForms.json is out of date (%v): run TestStubForms with POROS_UPDATE_FORMS=1", err)
	}
}
