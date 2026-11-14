//go:build windows

// Command othervpn stands in, inside poros-test, for another VPN client next to Poros, as Throne is on the user's
// computer: a TUN of its own with strict routes and FakeIP DNS, which sends what it takes straight out.
//
//	othervpn [-fakeip=true]
//
// It runs until it is killed; it needs administrator rights for its TUN.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"PorosCore/internal/boxmain"
)

func main() {
	fakeIP := flag.Bool("fakeip", true, "answer names with fake addresses, as Throne does")
	flag.Parse()
	dns := map[string]any{"servers": []map[string]any{{"type": "local", "tag": "local"}}}
	if *fakeIP {
		// sing-box takes no FakeIP as the default server: the rule sends addresses to it.
		dns = map[string]any{
			"servers": []map[string]any{
				{"type": "local", "tag": "local"},
				{"type": "fakeip", "tag": "fake", "inet4_range": "198.18.0.0/15", "inet6_range": "fc00::/18"},
			},
			"rules": []map[string]any{{"query_type": []string{"A", "AAAA"}, "server": "fake"}},
			"final": "local",
		}
	}
	config := map[string]any{
		"log": map[string]any{"level": "warn"},
		"dns": dns,
		// Throne's address: Poros keeps 172.19.1.1 for its own TUN.
		"inbounds": []map[string]any{{"type": "tun", "tag": "tun", "interface_name": "other-vpn",
			"address": []string{"172.19.0.1/24", "fdfe:dcba:9876::1/126"}, "auto_route": true, "strict_route": true}},
		"outbounds": []map[string]any{{"type": "direct", "tag": "direct"}},
		"route": map[string]any{
			"rules": []map[string]any{
				{"action": "sniff"},
				{"protocol": "dns", "action": "hijack-dns"},
				{"action": "resolve", "server": "local"},
			},
			"final":                 "direct",
			"auto_detect_interface": true,
		},
	}
	content, _ := json.Marshal(config)
	box, cancel, err := boxmain.Create(content, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "othervpn:", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "othervpn: up")
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	<-signals
	cancel()
	box.Close()
}
