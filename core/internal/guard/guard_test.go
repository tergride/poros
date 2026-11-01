package guard

import "testing"

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig(`{"allow_lan":true,"tun_name":"poros-net","tun_cidrs":["172.19.1.1/24"," "]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AllowLAN || cfg.TunName != "poros-net" || len(cfg.TunPrefixes) != 1 {
		t.Fatalf("parsed %+v", cfg)
	}
	for _, line := range []string{
		`{"allow_lan":1}`, `{"tun_name":"has space"}`, `{"tun_name":"longer-than-fifteen"}`, `{"tun_cidrs":["172.19.1.1"]}`,
	} {
		if _, err := parseConfig(line); errorCode(err) != CodeConfig {
			t.Fatalf("parseConfig(%s): error %v, want a %s error", line, err, CodeConfig)
		}
	}
}
