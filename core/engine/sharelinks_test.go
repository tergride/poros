package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"PorosCore/profile"
)

func TestShareAndExportServers(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	reality := "vless://" + testUUID + "@de1.example.com:443?security=reality&sni=www.example.com&fp=chrome" +
		"&pbk=Z84J2IelR9ch3k8VtlVhhs5ycBUlXA7wHBWcBrjqnAw&sid=ab&type=tcp#Germany"
	added, err := e.AddServers(AddServers{NewGroup: "Mine", Text: vless("fi.example.com", "Finland") + "\n" + reality})
	if err != nil || len(added.IDs) != 2 {
		t.Fatalf("AddServers: %+v, %v", added, err)
	}
	ws, real := added.IDs[0], added.IDs[1]

	links, err := e.ShareLinks(ServerIDs{IDs: []string{real, ws}})
	if err != nil || len(links.Links) != 2 || !strings.HasPrefix(links.Links[0], "vless://") {
		t.Fatalf("ShareLinks: %+v, %v", links, err)
	}
	deep, err := e.DeepLinks(ServerIDs{IDs: []string{real, ws}})
	if err != nil || len(deep.Links) != 2 {
		t.Fatalf("DeepLinks: %+v, %v", deep, err)
	}
	for i, id := range []string{real, ws} {
		stored, _ := e.Server(ByID{ID: id})
		for _, link := range []string{links.Links[i], deep.Links[i]} {
			if p, err := profile.ParseLink(link); err != nil || p.Name != stored.Profile.Name || p.Server != stored.Profile.Server {
				t.Errorf("%s reads as %+v, %v; want the server %s", link, p, err, stored.Server.Name)
			}
		}
		if !strings.HasPrefix(deep.Links[i], "poros://add/") {
			t.Errorf("a deep link: %s", deep.Links[i])
		}
	}
	if _, err = e.ShareLinks(ServerIDs{IDs: []string{ws, "missing"}}); reason(err) != ReasonNotFound {
		t.Errorf("a link of a missing server: %v", err)
	}

	for id, want := range map[string]string{real: CoreXray, ws: CoreSingBox} {
		out, err := e.ExportServer(ByID{ID: id})
		var outbound map[string]any
		if err != nil || out.Core != want || json.Unmarshal([]byte(out.JSON), &outbound) != nil {
			t.Errorf("ExportServer(%s): %+v, %v; want %s", id, out, err, want)
		}
	}
	settings, _ := e.Settings()
	settings.RealityOnXray = false
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if out, err := e.ExportServer(ByID{ID: real}); err != nil || out.Core != CoreSingBox {
		t.Errorf("REALITY with Xray turned off: %+v, %v", out, err)
	}
	if _, err := e.ExportServer(ByID{ID: "missing"}); reason(err) != ReasonNotFound {
		t.Errorf("exporting a missing server: %v", err)
	}
}
