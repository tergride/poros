package engine

import (
	"strings"
	"time"

	"PorosCore/config"
	"PorosCore/internal/jsonrpc"
)

const (
	defaultNTPServer   = "time.cloudflare.com"
	defaultNTPPort     = 123
	defaultNTPInterval = 30
	maxNTPInterval     = 24 * 60
)

// validateNTP checks the clock of the settings: a host name or an address, its port, and how often to ask.
func validateNTP(s Settings) error {
	if s.NTPOutbound != config.OutboundDirect && s.NTPOutbound != config.OutboundProxy {
		return jsonrpc.Failed(ReasonInvalidValue, "ntpOutbound must be direct or proxy, not %q", s.NTPOutbound)
	}
	server := strings.TrimSpace(s.NTPServer)
	if (s.NTP && server == "") || strings.ContainsAny(server, " /\\@") {
		return jsonrpc.Failed(ReasonInvalidValue, "the NTP server %q is no host name or address", s.NTPServer)
	}
	if s.NTPPort < 1 || s.NTPPort > 65535 {
		return jsonrpc.Failed(ReasonInvalidValue, "ntpPort must be from 1 to 65535, not %d", s.NTPPort)
	}
	if s.NTPInterval < 1 || s.NTPInterval > maxNTPInterval {
		return jsonrpc.Failed(ReasonInvalidValue, "ntpInterval must be from 1 to %d minutes, not %d", maxNTPInterval, s.NTPInterval)
	}
	return nil
}

// ntpOf is the clock of a connection, nil for the system's.
func ntpOf(s Settings) *config.NTP {
	if !s.NTP {
		return nil
	}
	return &config.NTP{
		Server:   strings.TrimSpace(s.NTPServer),
		Port:     uint16(s.NTPPort),
		Interval: time.Duration(s.NTPInterval) * time.Minute,
		Proxy:    s.NTPOutbound == config.OutboundProxy,
	}
}
