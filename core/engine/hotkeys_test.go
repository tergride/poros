package engine

import "testing"

func TestShortcutsAreCheckedAsAWhole(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	settings := DefaultSettings()
	if settings.MenuKeys.Search != "Ctrl+K" || settings.MenuKeys.Paste != "Ctrl+V" || settings.Hotkeys != (Hotkeys{}) {
		t.Fatalf("defaults: %+v %+v", settings.MenuKeys, settings.Hotkeys)
	}
	settings.Hotkeys = Hotkeys{Show: "Ctrl+Alt+P", Start: "Ctrl+Shift+Win+F12", Tun: "Win+Up"}
	settings.MenuKeys.Logs = "F7"
	settings.MenuKeys.Wizard = "Alt+Shift+1"
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatalf("good shortcuts: %v", err)
	}
	for name, change := range map[string]func(*Settings){
		"no modifier for Windows":    func(s *Settings) { s.Hotkeys.System = "P" },
		"Shift alone types":          func(s *Settings) { s.MenuKeys.Reset = "Shift+R" },
		"modifiers out of order":     func(s *Settings) { s.Hotkeys.Presets = "Alt+Ctrl+P" },
		"a modifier twice":           func(s *Settings) { s.Hotkeys.Presets = "Ctrl+Ctrl+P" },
		"no such key":                func(s *Settings) { s.Hotkeys.Presets = "Ctrl+Comma" },
		"lower case":                 func(s *Settings) { s.Hotkeys.Presets = "ctrl+p" },
		"global and window the same": func(s *Settings) { s.MenuKeys.Settings = "Ctrl+Alt+P" },
		"two window actions":         func(s *Settings) { s.MenuKeys.NewProfile = "Ctrl+K" },
	} {
		bad := settings
		change(&bad)
		if _, err := e.SetSettings(bad); err == nil {
			t.Errorf("%s is taken", name)
		}
	}
	bad := settings
	bad.MenuKeys.NewProfile = "Ctrl+K"
	if _, err := e.SetSettings(bad); reason(err) != ReasonDuplicate {
		t.Errorf("a shortcut twice: %v", err)
	}
}
