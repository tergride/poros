package engine

import (
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/sagernet/bbolt"

	"PorosCore/config"
	"PorosCore/internal/jsonrpc"
	"PorosCore/internal/logontask"
)

// DefaultSettings are what a new data folder starts with. TUN and System are both off: the first Start asks.
func DefaultSettings() Settings {
	return Settings{
		UIMode:                 "simple",
		Preset:                 "all",
		CheckIP:                true,
		IPv6:                   true,
		RealityOnXray:          true,
		MuxProtocol:            "smux",
		MuxConcurrency:         defaultMuxConcurrency,
		NTPServer:              defaultNTPServer,
		NTPPort:                defaultNTPPort,
		NTPInterval:            defaultNTPInterval,
		NTPOutbound:            config.OutboundDirect,
		LogLines:               defaultLogLines,
		Stats:                  true,
		TrafficHistory:         true,
		TunMTU:                 defaultTunMTU,
		TunStrictRoute:         true,
		TunIPv4:                TunIPv4Default,
		TunIPv6:                TunIPv6Default,
		TunBypass:              TunBypassDefault,
		LatencyTimeout:         int(defaultLatencyTimeout / time.Millisecond),
		LatencyConcurrency:     defaultLatencyConcurrency,
		DNSRouting:             DNSRoutingAuto,
		DNSHosts:               true,
		DNSCache:               true,
		DNSCacheSize:           defaultDNSCacheSize,
		DNSTimeout:             defaultDNSTimeout,
		WARPMode:               WARPModeWireGuard,
		SubscriptionAutoUpdate: true,
		ShowChanges:            true,
		AutoConnect:            true,
		CloseToTray:            true,
		KillSwitch:             KillSwitchOff,
		TextSize:               "normal",
		UpdateCheck:            true,
		SpeedTestMode:          SpeedTestFull,
		SpeedTestURL:           defaultSpeedTestURL,
		MenuKeys:               MenuKeys{Search: "Ctrl+K", Paste: "Ctrl+V"},
	}
}

// defaultSpeedTestURL is a download of 25 MB that answers fast from everywhere.
const defaultSpeedTestURL = "https://speed.cloudflare.com/__down?bytes=25000000"

// maxUpdateHours is a week: a longer interval is as good as no updates.
const maxUpdateHours = 7 * 24

func (e *Engine) Settings() (Settings, error) {
	settings := DefaultSettings()
	err := e.db.View(func(tx *bbolt.Tx) error {
		_, err := getJSON(tx, bucketSettings, keySettings, &settings)
		return err
	})
	return settings, err
}

// SetSettings replaces the settings as a whole and announces the result.
func (e *Engine) SetSettings(settings Settings) (Settings, error) {
	if err := e.validateSettings(settings); err != nil {
		return Settings{}, err
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	previous, err := e.Settings()
	if err != nil {
		return Settings{}, err
	}
	if settings.AdminAutostart != previous.AdminAutostart {
		if err = e.syncLogonTask(settings.AdminAutostart); err != nil {
			return Settings{}, err
		}
	}
	if err := e.db.Update(func(tx *bbolt.Tx) error {
		return putJSON(tx, bucketSettings, keySettings, settings)
	}); err != nil {
		return Settings{}, err
	}
	e.notify(EventSettingsChanged, settings)
	go e.syncSystemProxy()
	if settings.KillSwitch != previous.KillSwitch {
		go e.syncKillSwitch(previous.KillSwitch)
	} else if settings.Tun != previous.Tun || settings.Preset != previous.Preset || settings.AllowLAN != previous.AllowLAN || settings.IPv6 != previous.IPv6 ||
		dnsChanged(settings, previous) || proxyChanged(settings, previous) || tunChanged(settings, previous) {
		go e.restartRefusal()
	}
	switch {
	case settings.CheckIP && !previous.CheckIP:
		go e.requestNetworkCheck()
	case !settings.CheckIP && previous.CheckIP:
		e.clearNetworkCheck()
	}
	return settings, nil
}

// syncLogonTask creates the sign-in task when rights at sign-in come on, which takes administrator rights, and
// removes it when they go off: at once in a core with rights, else the task removes itself at the next sign-in.
func (e *Engine) syncLogonTask(on bool) error {
	program, err := os.Executable()
	if err != nil {
		return err
	}
	if on {
		if !elevated() {
			return jsonrpc.Failed(ReasonLogonTaskNeedsElevation, "the sign-in task takes administrator rights")
		}
		// The task would start the core for the other account.
		if e.foreignUser {
			return jsonrpc.Failed(ReasonLogonTaskFailed, "the core has the rights of another account")
		}
		if err = logontask.Register(program, e.dataDir); err != nil {
			log.Printf("sign-in task: %v", err)
			return jsonrpc.Failed(ReasonLogonTaskFailed, "%v", err)
		}
		log.Print("sign-in task: created")
		return nil
	}
	if elevated() && !e.foreignUser {
		if err = logontask.Remove(program, e.dataDir); err == nil {
			log.Print("sign-in task: removed")
			return nil
		}
		log.Printf("sign-in task: %v", err)
	}
	return logontask.MarkRemoval(e.dataDir)
}

func (e *Engine) validateSettings(settings Settings) error {
	if !slices.Contains([]string{"simple", "advanced"}, settings.UIMode) {
		return jsonrpc.Failed(ReasonInvalidValue, "uiMode must be simple or advanced, not %q", settings.UIMode)
	}
	if !slices.Contains([]string{"", "en", "ru"}, settings.Language) {
		return jsonrpc.Failed(ReasonInvalidValue, "language must be empty, en or ru, not %q", settings.Language)
	}
	if _, found, err := e.preset(settings.Preset); err != nil || !found {
		return jsonrpc.Failed(ReasonUnknownPreset, "there is no preset %q", settings.Preset)
	}
	if settings.Server != "" {
		if _, found, err := e.serverProfile(settings.Server); err != nil || !found {
			return jsonrpc.Failed(ReasonNotFound, "there is no server %q", settings.Server)
		}
	}
	if !slices.Contains([]string{KillSwitchOff, KillSwitchConnected, KillSwitchAlways}, settings.KillSwitch) {
		return jsonrpc.Failed(ReasonInvalidValue, "killSwitch must be off, connected or always, not %q", settings.KillSwitch)
	}
	if !slices.Contains([]string{"normal", "large"}, settings.TextSize) {
		return jsonrpc.Failed(ReasonInvalidValue, "textSize must be normal or large, not %q", settings.TextSize)
	}
	if settings.SubscriptionUpdateHours < 0 || settings.SubscriptionUpdateHours > maxUpdateHours {
		return jsonrpc.Failed(ReasonInvalidValue, "subscriptionUpdateHours must be from 0 to %d, not %d", maxUpdateHours, settings.SubscriptionUpdateHours)
	}
	if !slices.Contains([]string{"", "1.2", "1.3"}, settings.SubscriptionTLS) {
		return jsonrpc.Failed(ReasonInvalidValue, "subscriptionTls must be empty, 1.2 or 1.3, not %q", settings.SubscriptionTLS)
	}
	if !slices.Contains([]string{SpeedTestFull, SpeedTestDownload, SpeedTestUpload, SpeedTestSimple, SpeedTestCountry}, settings.SpeedTestMode) {
		return jsonrpc.Failed(ReasonInvalidValue, "unknown speed test mode %q", settings.SpeedTestMode)
	}
	if settings.SpeedTestMode == SpeedTestSimple && !strings.HasPrefix(settings.SpeedTestURL, "http://") && !strings.HasPrefix(settings.SpeedTestURL, "https://") {
		return jsonrpc.Failed(ReasonInvalidValue, "the simple speed test downloads an http:// or https:// address")
	}
	if !slices.Contains([]string{"", "1.1"}, settings.SubscriptionHTTP) {
		return jsonrpc.Failed(ReasonInvalidValue, "subscriptionHttp must be empty or 1.1, not %q", settings.SubscriptionHTTP)
	}
	if err := validateWARP(settings); err != nil {
		return err
	}
	if err := validateInbound(settings); err != nil {
		return err
	}
	if err := validateTun(settings); err != nil {
		return err
	}
	if err := validateProtocols(settings); err != nil {
		return err
	}
	if err := validateNTP(settings); err != nil {
		return err
	}
	if _, err := parseDeviceData(settings.DeviceData); err != nil {
		return jsonrpc.Failed(ReasonInvalidValue, "deviceData: %v", err)
	}
	if settings.LogLines < minLogLines || settings.LogLines > maxLogLines {
		return jsonrpc.Failed(ReasonInvalidValue, "logLines must be from %d to %d, not %d", minLogLines, maxLogLines, settings.LogLines)
	}
	if err := checkKeys(settings.Hotkeys, settings.MenuKeys); err != nil {
		return err
	}
	return validateDNS(settings)
}
