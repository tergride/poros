package engine

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"PorosCore/internal/jsonrpc"
)

// modifierOrder is how a shortcut writes its modifiers, so that one shortcut has one spelling.
var modifierOrder = []string{"Ctrl", "Alt", "Shift", "Win"}

// shortcutKey is the key of a shortcut: a letter, a digit, a function key or a named key.
var shortcutKey = regexp.MustCompile(`^([A-Z0-9]|F([1-9]|1[0-9]|2[0-4])|Up|Down|Left|Right|Home|End|PageUp|PageDown|Insert|Delete|Space|Enter|Tab|Backspace)$`)

// parseShortcut checks a shortcut as Hotkeys writes it; it tells whether it has a modifier, and whether that is
// Ctrl or Alt.
func parseShortcut(text string) (modifiers []string, key string, err error) {
	parts := strings.Split(text, "+")
	key = parts[len(parts)-1]
	modifiers = parts[:len(parts)-1]
	next := 0
	for _, m := range modifiers {
		at := -1
		for i := next; i < len(modifierOrder); i++ {
			if modifierOrder[i] == m {
				at = i
				break
			}
		}
		if at < 0 {
			return nil, "", fmt.Errorf("%q: modifiers are Ctrl, Alt, Shift and Win, each once, in that order", text)
		}
		next = at + 1
	}
	if !shortcutKey.MatchString(key) {
		return nil, "", fmt.Errorf("%q: %q is no key a shortcut takes", text, key)
	}
	return modifiers, key, nil
}

// checkKeys refuses a shortcut that is not one, a global one without a modifier, a shortcut of the window that
// would type in a field, and one shortcut for two actions.
func checkKeys(global Hotkeys, menu MenuKeys) error {
	taken := map[string]string{}
	check := func(name, text string, inWindow bool) error {
		if text == "" {
			return nil
		}
		modifiers, key, err := parseShortcut(text)
		if err != nil {
			return jsonrpc.Failed(ReasonInvalidValue, "%s: %v", name, err)
		}
		function := strings.HasPrefix(key, "F") && len(key) > 1
		switch {
		case !inWindow && len(modifiers) == 0:
			return jsonrpc.Failed(ReasonInvalidValue, "%s: %q needs Ctrl, Alt, Shift or Win", name, text)
		case inWindow && !function && !slices.Contains(modifiers, "Ctrl") && !slices.Contains(modifiers, "Alt"):
			return jsonrpc.Failed(ReasonInvalidValue, "%s: %q needs Ctrl or Alt, or a function key", name, text)
		}
		if other, found := taken[text]; found {
			return jsonrpc.Failed(ReasonDuplicate, "%s: %q is the shortcut of %s already", name, text, other)
		}
		taken[text] = name
		return nil
	}
	for _, k := range []struct {
		name, text string
		window     bool
	}{
		{"hotkeys.show", global.Show, false}, {"hotkeys.presets", global.Presets, false}, {"hotkeys.system", global.System, false},
		{"hotkeys.tun", global.Tun, false}, {"hotkeys.start", global.Start, false},
		{"menuKeys.search", menu.Search, true}, {"menuKeys.paste", menu.Paste, true}, {"menuKeys.newProfile", menu.NewProfile, true},
		{"menuKeys.scanQr", menu.ScanQR, true}, {"menuKeys.updateAll", menu.UpdateAll, true}, {"menuKeys.testGroup", menu.TestGroup, true},
		{"menuKeys.startStop", menu.StartStop, true}, {"menuKeys.reset", menu.Reset, true}, {"menuKeys.settings", menu.Settings, true},
		{"menuKeys.logs", menu.Logs, true}, {"menuKeys.wizard", menu.Wizard, true},
	} {
		if err := check(k.name, k.text, k.window); err != nil {
			return err
		}
	}
	return nil
}
