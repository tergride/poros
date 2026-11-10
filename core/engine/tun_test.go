package engine

import (
	"slices"
	"testing"

	"PorosCore/config"
)

// The TUN of the settings: as it comes it is the TUN the config builds without settings, and what would break it is
// refused.
func TestTunSettings(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	settings, err := e.Settings()
	if err != nil {
		t.Fatal(err)
	}
	tun := tunOf(settings)
	if tun.MTU != 1500 || tun.LooseRoute || tun.Routing || tun.Address4.String() != "172.19.1.1/24" || !slices.Equal(tun.Bypass, config.PrivateRanges()) {
		t.Errorf("the TUN as it comes: %+v", tun)
	}
	for name, change := range map[string]func(s *Settings){
		"a small MTU":         func(s *Settings) { s.TunMTU = 1000 },
		"a network's address": func(s *Settings) { s.TunIPv4 = "172.19.1.0/24" },
		"IPv6 for IPv4":       func(s *Settings) { s.TunIPv4 = "fdfe::1/96" },
		"every address":       func(s *Settings) { s.TunBypass = "10.0.0.0/8\n0.0.0.0/0" },
		"no subnet":           func(s *Settings) { s.TunBypass = "lan" },
	} {
		wrong := settings
		change(&wrong)
		if _, err := e.SetSettings(wrong); reason(err) != ReasonInvalidValue {
			t.Errorf("%s: %v", name, err)
		}
	}
	settings.TunMTU, settings.TunStrictRoute, settings.TunRouting, settings.TunBypass = 9000, false, true, "# home\n192.168.50.1\n"
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if tun = tunOf(settings); tun.MTU != 9000 || !tun.LooseRoute || !tun.Routing || len(tun.Bypass) != 1 || tun.Bypass[0].String() != "192.168.50.1/32" {
		t.Errorf("the TUN of the settings: %+v", tun)
	}
}
