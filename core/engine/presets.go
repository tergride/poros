package engine

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed presets/*.json
var presetFiles embed.FS

// The order of the mockups: everything, then narrower, then the hardened one.
var builtinPresetIDs = []string{"all", "except-ru", "blocked-only", "hardened"}

const presetKind = "poros-route-profile"

func builtinPresets() ([]Preset, error) {
	presets := make([]Preset, 0, len(builtinPresetIDs))
	for _, id := range builtinPresetIDs {
		data, err := presetFiles.ReadFile("presets/" + id + ".json")
		if err != nil {
			return nil, err
		}
		var head struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		}
		if err = json.Unmarshal(data, &head); err != nil {
			return nil, fmt.Errorf("preset %s: %w", id, err)
		}
		if head.Kind != presetKind || head.Name == "" {
			return nil, fmt.Errorf("preset %s: not a %s with a name", id, presetKind)
		}
		presets = append(presets, Preset{ID: id, Name: head.Name, Builtin: true})
	}
	return presets, nil
}

func (e *Engine) Presets() []Preset {
	return append([]Preset(nil), e.presets...)
}
