package config

import (
	"testing"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	R "github.com/sagernet/sing-box/route/rule"
	M "github.com/sagernet/sing/common/metadata"
)

// A program matches by its name and by its path whatever their case, in routing and in DNS alike, and a rule of
// several conditions still needs all of them.
func TestProgramsMatchInAnyCase(t *testing.T) {
	for _, c := range []struct {
		what  string
		rule  Rule
		path  string
		match bool
	}{
		{"a name in another case", Rule{ProcessName: []string{"game.exe"}}, `C:\Apps\Game.exe`, true},
		{"a file in upper case", Rule{ProcessName: []string{"ping.exe"}}, `C:\Windows\System32\PING.EXE`, true},
		{"a second name", Rule{ProcessName: []string{"app.exe", "player.exe"}}, `C:\Apps\Player.exe`, true},
		{"a Cyrillic name", Rule{ProcessName: []string{"игра.exe"}}, `C:\Игры\ИГРА.exe`, true},
		{"a name with signs", Rule{ProcessName: []string{"notepad++ (x64).exe"}}, `C:\Apps\Notepad++ (x64).exe`, true},
		{"a longer name", Rule{ProcessName: []string{"app.exe"}}, `C:\Apps\MyApp.exe`, false},
		{"a longer extension", Rule{ProcessName: []string{"app.exe"}}, `C:\Apps\app.exe.bak`, false},
		{"a dot is only a dot", Rule{ProcessName: []string{"app.exe"}}, `C:\Apps\appXexe`, false},
		{"a folder of that name", Rule{ProcessName: []string{"app.exe"}}, `C:\app.exe\other.exe`, false},
		{"a path in another case", Rule{ProcessPath: []string{`c:\apps\game.exe`}}, `C:\Apps\Game.exe`, true},
		{"a path in another folder", Rule{ProcessPath: []string{`C:\Apps\game.exe`}}, `D:\C:\Apps\game.exe`, false},
		{"a name and its path", Rule{ProcessName: []string{"game.exe"}, ProcessPath: []string{`c:\apps\game.exe`}}, `C:\Apps\Game.exe`, true},
		{"a name on another path", Rule{ProcessName: []string{"game.exe"}, ProcessPath: []string{`c:\apps\game.exe`}}, `D:\Other\Game.exe`, false},
		{"a path of another name", Rule{ProcessName: []string{"other.exe"}, ProcessPath: []string{`c:\apps\game.exe`}}, `C:\Apps\Game.exe`, false},
		{"a name and a pattern", Rule{ProcessName: []string{"game.exe"}, ProcessPathRegex: []string{`^C:\\Apps\\`}}, `C:\Apps\Game.exe`, true},
		{"a name outside the pattern", Rule{ProcessName: []string{"game.exe"}, ProcessPathRegex: []string{`^C:\\Apps\\`}}, `D:\Game.exe`, false},
		{"a pattern keeps its case", Rule{ProcessPathRegex: []string{`game\.exe$`}}, `C:\Apps\Game.exe`, false},
		{"all but a name", Rule{Invert: true, ProcessName: []string{"game.exe"}}, `C:\Apps\Game.exe`, false},
		{"all but a name and a path", Rule{Invert: true, ProcessName: []string{"game.exe"}, ProcessPath: []string{`c:\apps\game.exe`}}, `D:\Game.exe`, true},
	} {
		t.Run(c.what, func(t *testing.T) {
			rule := c.rule
			rule.Action, rule.Outbound = ActionRoute, OutboundDirect
			configs, err := Build(Input{
				Server:    mustParse(t, sampleLinks(t)["trojan tcp"]),
				Preset:    Preset{Name: "Programs", DefaultOutbound: OutboundProxy, Rules: []Rule{rule}},
				MixedPort: 2090,
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx, logger := include.Context(t.Context()), log.NewNOPFactory().Logger()
			routes := configs.SingBox.Route.Rules
			route, err := R.NewRule(ctx, logger, routes[len(routes)-1], false)
			if err != nil {
				t.Fatal(err)
			}
			queries := configs.SingBox.DNS.Rules
			if len(queries) != 1 {
				t.Fatalf("DNS rules: %+v", queries)
			}
			query, err := R.NewDNSRule(ctx, logger, queries[0], false, false)
			if err != nil {
				t.Fatal(err)
			}
			program := func() *adapter.InboundContext {
				return &adapter.InboundContext{
					Network:     "tcp",
					Destination: M.ParseSocksaddr("198.51.100.7:443"),
					ProcessInfo: &adapter.ConnectionOwner{ProcessPaths: []string{c.path}},
				}
			}
			if got := route.Match(program()); got != c.match {
				t.Errorf("route: %v, want %v", got, c.match)
			}
			if got := query.LegacyPreMatch(program()); got != c.match {
				t.Errorf("DNS: %v, want %v", got, c.match)
			}
		})
	}
}
