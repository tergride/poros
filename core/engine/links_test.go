package engine

import (
	"encoding/base64"
	"testing"
)

func TestOutsideLinks(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	encode := func(text string) string { return base64.RawURLEncoding.EncodeToString([]byte(text)) }

	info, err := e.ParseLink(OutsideLink{Link: "poros://AddSub/" + encode("https://sub.example.com/u/8f3a#My%20VPN")})
	if err != nil || info.Kind != LinkSubscription || info.URL != "https://sub.example.com/u/8f3a" || info.Name != "My VPN" {
		t.Fatalf("subscription link: %+v, %v", info, err)
	}
	padded := base64.StdEncoding.EncodeToString([]byte("https://vpn.example.org/s/71bd"))
	if info, err = e.ParseLink(OutsideLink{Link: "throne://addsub/" + padded}); err != nil || info.URL != "https://vpn.example.org/s/71bd" || info.Name != "" {
		t.Fatalf("Throne subscription link: %+v, %v", info, err)
	}

	draft, _ := e.PresetCopy("all")
	draft.Name = "Home"
	draft.Sites = []PresetEntry{{Value: "example.org", Action: ActionBlock}}
	draft.DNS = "tls://1.1.1.1"
	saved, err := e.SavePreset(draft)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := e.ExportPreset(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	info, err = e.ParseLink(OutsideLink{Link: exported.Link})
	if err != nil || info.Kind != LinkPreset || info.Preset == nil || info.Preset.Name != "Home" || info.Preset.BasedOn != "all" ||
		len(info.Preset.Sites) != 1 || info.Preset.DNS != "tls://1.1.1.1" {
		t.Fatalf("preset link: %+v, %v", info, err)
	}
	if presets, _ := e.Presets(); len(presets) != 3 {
		t.Fatalf("reading a link added a preset: %d presets", len(presets))
	}

	server := `{"type":"trojan","name":"🇩🇪 Home","server":"de.example.com","server_port":443,"trojan":{"password":"p","tls":{"enabled":true}}}`
	info, err = e.ParseLink(OutsideLink{Link: "poros://add/" + encode(server)})
	if err != nil || info.Kind != LinkServer || info.Server == nil ||
		*info.Server != (Server{Name: "🇩🇪 Home", Protocol: "trojan", Host: "de.example.com", Port: 443, Country: "DE"}) {
		t.Fatalf("server link: %+v, %v", info, err)
	}

	throne := `{"type":"trojan","tag":"Throne","server":"th.example.com","server_port":8443,"password":"p"}`
	info, err = e.ParseLink(OutsideLink{Link: "throne://add/" + encode(throne)})
	if err != nil || info.Kind != LinkServer || info.Server == nil ||
		*info.Server != (Server{Name: "Throne", Protocol: "trojan", Host: "th.example.com", Port: 8443}) {
		t.Fatalf("Throne server link: %+v, %v", info, err)
	}

	for link, want := range map[string]string{
		"poros://add/" + encode(`{"type":"vless"}`): ReasonInvalidLink,
		"poros://add/%%%": ReasonInvalidLink,
		"throne://add/" + encode(`{"type":"direct"}`):     ReasonInvalidLink,
		"poros://addsub/" + encode("vless://id@host:443"): ReasonSingleServer,
		"poros://addsub/" + encode("ftp://example.com/x"): ReasonInvalidURL,
		"poros://addsub/%%%":                              ReasonInvalidLink,
		"throne://route/" + encode("{}"):                  ReasonInvalidLink,
		"poros://remoteroute/x":                           ReasonInvalidLink,
		"https://sub.example.com/u/8f3a":                  ReasonInvalidLink,
	} {
		if _, err := e.ParseLink(OutsideLink{Link: link}); reason(err) != want {
			t.Errorf("%s: %v, want %s", link, err, want)
		}
	}
	if err := checkSubscriptionURL("poros://add/" + encode("{}")); reason(err) != ReasonSingleServer {
		t.Errorf("a poros://add/ link pasted as a subscription: %v", err)
	}
}
