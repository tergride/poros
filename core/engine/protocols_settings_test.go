package engine

import (
	"testing"
)

// «Protocols and bypass»: the defaults of profiles and QUIC, what is refused, and the tuning of a connection.
func TestProtocolSettings(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	settings, err := e.Settings()
	if err != nil || settings.MuxDefault || settings.MuxProtocol != "smux" || settings.MuxConcurrency != 8 || settings.FragmentDefault || settings.SpoofDefault {
		t.Fatalf("the defaults: %+v, %v", settings, err)
	}
	for name, change := range map[string]func(s *Settings){
		"a mux protocol":         func(s *Settings) { s.MuxProtocol = "mux.cool" },
		"no streams":             func(s *Settings) { s.MuxConcurrency = 0 },
		"a fingerprint":          func(s *Settings) { s.FingerprintDefault = "netscape" },
		"a spoof method":         func(s *Settings) { s.SpoofMethod = "wrong-everything" },
		"a spoof without a name": func(s *Settings) { s.SpoofDefault = true },
		"an idle timeout":        func(s *Settings) { s.QUICIdleTimeout = "soon" },
		"a window":               func(s *Settings) { s.QUICStreamWindow = "big" },
		"a small packet":         func(s *Settings) { s.QUICInitialPacketSize = 100 },
	} {
		wrong := settings
		change(&wrong)
		if _, err := e.SetSettings(wrong); reason(err) != ReasonInvalidValue {
			t.Errorf("%s: %v", name, err)
		}
	}
	settings.MuxDefault, settings.MuxProtocol, settings.MuxConcurrency = true, "yamux", 4
	settings.FragmentDefault, settings.FingerprintDefault = true, "chrome"
	settings.SpoofDefault, settings.SpoofSNI, settings.SpoofMethod = true, " www.example.com ", "wrong-ack"
	settings.QUICIdleTimeout, settings.QUICStreamWindow, settings.QUICInitialPacketSize = "30s", "8MB", 1300
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	tune := tuningOf(settings)
	if !tune.Multiplex || tune.MuxProtocol != "yamux" || tune.MuxStreams != 4 || !tune.Fragment || tune.Fingerprint != "chrome" ||
		!tune.Spoof || tune.SpoofSNI != "www.example.com" || tune.SpoofMethod != "wrong-ack" ||
		tune.QUIC.StreamReceiveWindow == nil || tune.QUIC.StreamReceiveWindow.Value() != 8<<20 || tune.QUIC.InitialPacketSize != 1300 {
		t.Errorf("the tuning of a connection: %+v", tune)
	}
}
