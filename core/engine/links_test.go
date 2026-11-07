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

	for link, want := range map[string]string{
		"poros://add/" + encode(`{"type":"vless"}`):       ReasonSingleServer,
		"throne://add/" + encode(`{"type":"vless"}`):      ReasonSingleServer,
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
