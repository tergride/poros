package engine

import (
	"slices"
	"strings"

	"PorosCore/config"
	"PorosCore/internal/jsonrpc"
)

// The choices of «Protocols and bypass»: the multiplex protocols of sing-box, the uTLS fingerprints, and how a
// server drops a forged handshake ("" is the core's).
var (
	muxProtocols = []string{"smux", "yamux", "h2mux"}
	fingerprints = []string{"", "chrome", "firefox", "edge", "safari", "360", "qq", "ios", "android", "random", "randomized"}
	spoofMethods = []string{"", "wrong-sequence", "wrong-checksum", "wrong-ack", "wrong-md5", "wrong-timestamp"}
)

const (
	defaultMuxConcurrency = 8
	maxMuxConcurrency     = 128
)

// validateProtocols checks the defaults of the profiles and the QUIC options of the settings.
func validateProtocols(s Settings) error {
	if !slices.Contains(muxProtocols, s.MuxProtocol) {
		return jsonrpc.Failed(ReasonInvalidValue, "muxProtocol must be smux, yamux or h2mux, not %q", s.MuxProtocol)
	}
	if s.MuxConcurrency < 1 || s.MuxConcurrency > maxMuxConcurrency {
		return jsonrpc.Failed(ReasonInvalidValue, "muxConcurrency must be from 1 to %d, not %d", maxMuxConcurrency, s.MuxConcurrency)
	}
	if !slices.Contains(fingerprints, s.FingerprintDefault) {
		return jsonrpc.Failed(ReasonInvalidValue, "unknown uTLS fingerprint %q", s.FingerprintDefault)
	}
	if !slices.Contains(spoofMethods, s.SpoofMethod) {
		return jsonrpc.Failed(ReasonInvalidValue, "unknown spoof method %q", s.SpoofMethod)
	}
	if s.SpoofDefault && strings.TrimSpace(s.SpoofSNI) == "" {
		return jsonrpc.Failed(ReasonInvalidValue, "a forged handshake by default needs the name to forge")
	}
	if _, err := quicOf(s); err != nil {
		return jsonrpc.Failed(ReasonInvalidValue, "%v", err)
	}
	return nil
}

func quicOf(s Settings) (config.Tuning, error) {
	quic, err := config.ParseQUIC(s.QUICIdleTimeout, s.QUICKeepAlive, s.QUICStreamWindow, s.QUICConnectionWindow,
		s.QUICMaxStreams, s.QUICInitialPacketSize, s.QUICDisablePMTUD)
	return config.Tuning{QUIC: quic}, err
}

// tuningOf is what the settings add to the servers of a connection, which validateProtocols has checked.
func tuningOf(s Settings) config.Tuning {
	tune, _ := quicOf(s)
	tune.Multiplex, tune.MuxProtocol, tune.MuxStreams = s.MuxDefault, s.MuxProtocol, s.MuxConcurrency
	tune.Fragment, tune.Fingerprint = s.FragmentDefault, s.FingerprintDefault
	tune.Spoof, tune.SpoofSNI, tune.SpoofMethod = s.SpoofDefault, strings.TrimSpace(s.SpoofSNI), s.SpoofMethod
	return tune
}
