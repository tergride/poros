package engine

import (
	"embed"
	"fmt"

	"PorosCore/config"
)

//go:embed presets/*.json
var presetFiles embed.FS

// The order of the mockups: everything, then the hardened one.
var builtinPresetIDs = []string{"all", "hardened"}

type presetRecord struct {
	ID      string
	Builtin bool
	Preset  config.Preset
}

func (r presetRecord) view() Preset {
	return Preset{ID: r.ID, Name: r.Preset.Name, Builtin: r.Builtin}
}

func builtinPresets() ([]presetRecord, error) {
	presets := make([]presetRecord, 0, len(builtinPresetIDs))
	for _, id := range builtinPresetIDs {
		data, err := presetFiles.ReadFile("presets/" + id + ".json")
		if err != nil {
			return nil, err
		}
		preset, err := config.ParsePreset(data)
		if err != nil {
			return nil, fmt.Errorf("preset %s: %w", id, err)
		}
		presets = append(presets, presetRecord{ID: id, Builtin: true, Preset: preset})
	}
	return presets, nil
}

func (e *Engine) Presets() []Preset {
	views := make([]Preset, len(e.presets))
	for i, preset := range e.presets {
		views[i] = preset.view()
	}
	return views
}
