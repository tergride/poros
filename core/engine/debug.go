package engine

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

// hiddenSettings may carry passwords, keys, the addresses of the user's own servers or what they visit: the debug
// information says only whether they are set.
var hiddenSettings = []string{
	"proxyUsername", "proxyPassword", "dnsRemote", "dnsDirect", "dnsAnswers", "dnsClientSubnet", "dnsObject",
	"warpWireGuard", "warpMasque", "warpHosts", "deviceData", "subscriptionUserAgent", "latencyUrl", "speedTestUrl",
	"spoofSni", "ntpServer",
}

// DebugInfo is the text the user pastes into a report: the versions, the system, the state of the connection and
// the settings. It leaves out links, keys, addresses and names of servers, and the text of errors, which may name
// them.
func (e *Engine) DebugInfo() (string, error) {
	settings, err := e.Settings()
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return "", err
	}
	var fields map[string]any
	if err = json.Unmarshal(data, &fields); err != nil {
		return "", err
	}
	for _, name := range hiddenSettings {
		if value, ok := fields[name]; ok && !blank(value) {
			fields[name] = "(set)"
		}
	}
	shown, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return "", err
	}
	info := e.Info()
	version := e.UpdatesState().Current
	if version == "" {
		version = "local build"
	}
	groups, _ := e.Groups()
	servers := 0
	for _, g := range groups {
		servers += g.Servers
	}
	state := e.ConnectionState()
	var text strings.Builder
	fmt.Fprintf(&text, "Poros %s\n", version)
	fmt.Fprintf(&text, "sing-box %s, Xray %s, %s\n", info.SingBox, info.Xray, runtime.Version())
	fmt.Fprintf(&text, "%s %s, %s, administrator rights: %t\n", osName, osVersion(), runtime.GOARCH, info.Elevated)
	fmt.Fprintf(&text, "Connection: %s", state.State)
	if state.Error != "" {
		fmt.Fprintf(&text, ", error %s", state.Error)
	}
	fmt.Fprintf(&text, "\nGroups: %d, servers: %d\n", len(groups), servers)
	fmt.Fprintf(&text, "Settings: %s\n", shown)
	return text.String(), nil
}

// blank is a value left as it comes: empty text, zero, false or an object of such values.
func blank(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case float64:
		return v == 0
	case bool:
		return !v
	case map[string]any:
		for _, inner := range v {
			if !blank(inner) {
				return false
			}
		}
		return true
	case []any:
		return len(v) == 0
	}
	return false
}
