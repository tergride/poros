package engine

import (
	"log"
	"os"
	"slices"

	"github.com/sagernet/bbolt"

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
		SubscriptionAutoUpdate: true,
		ShowChanges:            true,
		AutoConnect:            true,
		CloseToTray:            true,
		KillSwitch:             KillSwitchOff,
		TextSize:               "normal",
		UpdateCheck:            true,
	}
}

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
	return nil
}
