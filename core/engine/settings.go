package engine

import (
	"slices"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/jsonrpc"
)

// DefaultSettings are what a new data folder starts with. TUN and System are both off: the first Start asks.
func DefaultSettings() Settings {
	return Settings{
		UIMode:                 "simple",
		Preset:                 "all",
		CheckIP:                true,
		SubscriptionAutoUpdate: true,
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
	if err := e.db.Update(func(tx *bbolt.Tx) error {
		return putJSON(tx, bucketSettings, keySettings, settings)
	}); err != nil {
		return Settings{}, err
	}
	e.notify(EventSettingsChanged, settings)
	return settings, nil
}

func (e *Engine) validateSettings(settings Settings) error {
	if !slices.Contains([]string{"simple", "advanced"}, settings.UIMode) {
		return jsonrpc.Failed(ReasonInvalidValue, "uiMode must be simple or advanced, not %q", settings.UIMode)
	}
	if !slices.Contains([]string{"", "en", "ru"}, settings.Language) {
		return jsonrpc.Failed(ReasonInvalidValue, "language must be empty, en or ru, not %q", settings.Language)
	}
	if !slices.ContainsFunc(e.presets, func(p Preset) bool { return p.ID == settings.Preset }) {
		return jsonrpc.Failed(ReasonUnknownPreset, "there is no preset %q", settings.Preset)
	}
	if settings.SubscriptionUpdateHours < 0 || settings.SubscriptionUpdateHours > maxUpdateHours {
		return jsonrpc.Failed(ReasonInvalidValue, "subscriptionUpdateHours must be from 0 to %d, not %d", maxUpdateHours, settings.SubscriptionUpdateHours)
	}
	return nil
}
