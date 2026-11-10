package engine

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"PorosCore/config"
	"PorosCore/internal/jsonrpc"
	"PorosCore/internal/warp"
)

// registerTimeout bounds a registration at Cloudflare, every host of the API included.
const registerTimeout = 60 * time.Second

func deviceOf(d WARPDevice) config.WARPDevice {
	return config.WARPDevice{Endpoint: d.Endpoint, PrivateKey: d.PrivateKey, PeerKey: d.PeerKey, Addresses: d.Addresses, Reserved: d.Reserved, ServerName: d.ServerName, HTTP: d.HTTP}
}

// validateWARP checks what is written of both devices, and that the device of the mode is whole while WARP is on.
func validateWARP(s Settings) error {
	if s.WARPMode != WARPModeWireGuard && s.WARPMode != WARPModeMASQUE {
		return jsonrpc.Failed(ReasonInvalidValue, "warpMode must be wireguard or masque, not %q", s.WARPMode)
	}
	for mode, device := range map[string]WARPDevice{WARPModeWireGuard: s.WARPWireGuard, WARPModeMASQUE: s.WARPMASQUE} {
		if _, err := config.ParseWARP(mode, deviceOf(device), s.WARP && mode == s.WARPMode); err != nil {
			return jsonrpc.Failed(ReasonInvalidValue, "%v", err)
		}
	}
	return nil
}

// warpOf is the WARP of a connection by the settings, which validateWARP has checked; nil while WARP is off.
func warpOf(s Settings) *config.WARP {
	if !s.WARP {
		return nil
	}
	device := s.WARPWireGuard
	if s.WARPMode == WARPModeMASQUE {
		device = s.WARPMASQUE
	}
	w, _ := config.ParseWARP(s.WARPMode, deviceOf(device), true)
	return w
}

// RegisterWARP gets a new device of WARP from Cloudflare and keeps it for its mode. While a connection runs, the
// request goes through its server, so that a provider that blocks Cloudflare does not stop it.
func (e *Engine) RegisterWARP(ctx context.Context, r WARPRegister) (Settings, error) {
	if r.Mode != WARPModeWireGuard && r.Mode != WARPModeMASQUE {
		return Settings{}, jsonrpc.Failed(ReasonInvalidValue, "mode must be wireguard or masque, not %q", r.Mode)
	}
	settings, err := e.Settings()
	if err != nil {
		return Settings{}, err
	}
	var client *http.Client
	if box := e.conn.box.Load(); box != nil && e.ConnectionState().State == ConnectionRunning {
		var done func()
		client, done = outboundClient(box, config.TagProxy, "", 0)
		defer done()
	}
	ctx, cancel := context.WithTimeout(ctx, registerTimeout)
	defer cancel()
	identity, err := warp.Register(ctx, r.Mode, client, strings.Split(settings.WARPHosts, "\n"))
	if err != nil {
		return Settings{}, jsonrpc.Failed(ReasonWARPFailed, "%v", err)
	}
	var addresses []string
	for _, address := range []string{identity.IPv4, identity.IPv6} {
		if address != "" && !strings.Contains(address, "/") {
			if strings.Contains(address, ":") {
				address += "/128"
			} else {
				address += "/32"
			}
		}
		if address != "" {
			addresses = append(addresses, address)
		}
	}
	device := WARPDevice{
		Endpoint: identity.Endpoint, PrivateKey: identity.PrivateKey, PeerKey: identity.PeerPublicKey,
		Addresses: strings.Join(addresses, ", "), RegisteredAt: e.now().UnixMilli(),
	}
	if r.Mode == WARPModeWireGuard {
		reserved := make([]string, len(identity.Reserved))
		for i, b := range identity.Reserved {
			reserved[i] = strconv.Itoa(int(b))
		}
		device.Reserved = strings.Join(reserved, ", ")
	}
	// The settings may have changed while Cloudflare answered.
	if settings, err = e.Settings(); err != nil {
		return Settings{}, err
	}
	if r.Mode == WARPModeWireGuard {
		settings.WARPWireGuard = device
	} else {
		device.ServerName, device.HTTP = settings.WARPMASQUE.ServerName, settings.WARPMASQUE.HTTP
		settings.WARPMASQUE = device
	}
	return e.SetSettings(settings)
}
