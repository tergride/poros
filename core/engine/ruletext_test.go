package engine

import (
	"sync/atomic"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/trafficcontrol"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	R "github.com/sagernet/sing-box/route/rule"
	M "github.com/sagernet/sing/common/metadata"
)

// A service has two paths, of its DLL and of the program that hosts it: a pattern shows the one it found.
func TestRuleTextShowsTheMatchedPath(t *testing.T) {
	paths := []string{`C:\Windows\System32\dnsrslvr.dll`, `C:\Windows\System32\svchost.exe`}
	pattern := func(values ...string) option.Rule {
		return option.Rule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{
			RawDefaultRule: option.RawDefaultRule{ProcessPathRegex: values},
		}}
	}
	withPort := pattern(`svchost\.exe$`)
	withPort.DefaultOptions.Port = []uint16{443}
	inverted := pattern(`other\.exe$`)
	inverted.DefaultOptions.Invert = true
	both := option.Rule{Type: C.RuleTypeLogical, LogicalOptions: option.LogicalRule{
		RawLogicalRule: option.RawLogicalRule{Mode: C.LogicalTypeAnd, Rules: []option.Rule{
			{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: option.RawDefaultRule{ProcessName: []string{"svchost.exe"}}}},
			pattern(`svchost\.exe$`),
		}},
	}}

	for _, c := range []struct {
		what string
		rule option.Rule
		want string
	}{
		{"the host", pattern(`(?i)SVCHOST\.EXE$`), "process_name=svchost.exe"},
		{"the DLL", pattern(`dnsrslvr\.dll$`), "process_name=dnsrslvr.dll"},
		{"a pattern the text leaves out", pattern(`a( b)?\.exe$`, `c\.exe$`, `d\.exe$`, `svchost\.exe$`), "process_name=svchost.exe"},
		{"after other conditions", withPort, "port=443 process_name=svchost.exe"},
		{"with a name", both, "process_name=svchost.exe && process_name=svchost.exe"},
		{"all but a pattern", inverted, `!(process_path_regex=other\.exe$)`},
	} {
		options := c.rule
		action := option.RuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.RouteActionOptions{Outbound: "direct"}}
		options.DefaultOptions.RuleAction, options.LogicalOptions.RuleAction = action, action
		rule, err := R.NewRule(include.Context(t.Context()), log.NewNOPFactory().Logger(), options, false)
		if err != nil {
			t.Fatal(err)
		}
		shown := connectionOf(&trafficcontrol.TrackerMetadata{
			Metadata: adapter.InboundContext{
				Network:     "tcp",
				Destination: M.ParseSocksaddr("198.51.100.7:443"),
				ProcessInfo: &adapter.ConnectionOwner{ProcessPaths: paths},
			},
			Rule:   rule,
			Upload: new(atomic.Int64), Download: new(atomic.Int64),
		})
		if shown.Rule != c.want || shown.ProcessPath != paths[0] {
			t.Errorf("%s: %q of %q, want %q", c.what, shown.Rule, shown.ProcessPath, c.want)
		}
	}
}
