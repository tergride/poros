package config

import "testing"

// The text of a rule shows programs whatever signs their names have, and leaves other rules as they are.
func TestRuleText(t *testing.T) {
	names := func(values ...string) string { return patternItem + nameStart + anyOf(values) + patternEnd }
	paths := func(values ...string) string { return patternItem + pathStart + anyOf(values) + patternEnd }
	const signs = `a.b+c (x64) [1] {2} $^ 100% & Игра.exe`

	for _, c := range []struct {
		what, text string
		program    []string
		want       string
	}{
		{"the file's case", names("game.exe"), []string{`C:\Apps\Game.exe`}, "process_name=Game.exe"},
		{"the one of several", names("app.exe", "player.exe"), []string{`C:\Apps\Player.exe`}, "process_name=Player.exe"},
		{"signs", names(signs), []string{`C:\Apps\` + signs}, "process_name=" + signs},
		{"signs as written", names(signs, "b|c).exe"), nil, "process_name=[" + signs + " b|c).exe]"},
		{"the path", paths(`c:\program files (x86)\app.exe`), []string{`C:\Program Files (x86)\App.exe`}, `process_path=C:\Program Files (x86)\App.exe`},
		{"the program behind its service", names("svchost.exe"), []string{`C:\Windows\System32\dnsrslvr.dll`, `C:\Windows\System32\svchost.exe`}, "process_name=svchost.exe"},
		{"after other conditions", "network=tcp port=443 " + names("game.exe"), []string{`C:\Game.exe`}, "network=tcp port=443 process_name=Game.exe"},
		{"a name with &&", names("tom && jerry.exe") + " && " + paths(`c:\tom && jerry.exe`), []string{`C:\Tom && Jerry.exe`}, `process_name=Tom && Jerry.exe && process_path=C:\Tom && Jerry.exe`},
		{"all but a name", "!(" + names("game.exe") + ")", []string{`C:\Other.exe`}, "!(process_name=game.exe)"},
		{"all but a name and a path", "!(" + names("game.exe") + " && " + paths(`c:\game.exe`) + ")", []string{`D:\Game.exe`}, `!(process_name=game.exe && process_path=c:\game.exe)`},
		{"the user's pattern", "port=443 " + patternItem + `[(?i)\\(game|game-helper)\.exe$ (?i)\\(player( beta)?)\.exe$]`, []string{`C:\Apps\Game.exe`}, "port=443 process_name=Game.exe"},
		{"a name and the user's pattern", names("game.exe") + " && " + patternItem + `^C:\\Games\\`, []string{`C:\Games\Game.exe`}, "process_name=Game.exe && process_name=Game.exe"},
		{"all but the user's pattern", "!(" + patternItem + `game\.exe$)`, []string{`C:\Other.exe`}, "!(" + patternItem + `game\.exe$)`},
		{"no program known", patternItem + `game\.exe$`, nil, patternItem + `game\.exe$`},
		{"a rule on sites", "domain_suffix=[example.com example.org]", []string{`C:\Apps\App.exe`}, "domain_suffix=[example.com example.org]"},
	} {
		if got := RuleText(c.text, c.program); got != c.want {
			t.Errorf("%s: %q, want %q", c.what, got, c.want)
		}
	}
}
