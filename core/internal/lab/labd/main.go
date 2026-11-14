// Command labd runs the services of the leak lab on its gateway (Alpine Linux, see vm\gw in the workspace).
//
//	labd ca <dir>     makes the lab's certificate authority in dir, once per start of the gateway
//	labd inet <dir>   the lab's internet: sites that echo who asks, DNS resolvers, DoH, STUN, and stand-ins for the
//	                  services Poros asks itself (IP checks, the DNS leak test, the delay and speed tests)
//	labd vpn <dir>    the lab's VPN servers, and the subscription that lists them
//
// The gateway's poros-lab script runs inet and vpn each in its own network namespace, so that the internet sees the
// address a packet really comes from: the user's, behind the provider's NAT, or a VPN server's. Every request is
// logged as a JSON line, which the host reads after each scenario.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: labd ca|inet|vpn <state dir>")
		os.Exit(2)
	}
	dir := os.Args[2]
	var err error
	switch os.Args[1] {
	case "ca":
		err = makeCA(dir)
	case "inet":
		err = runInet(dir)
	case "vpn":
		err = runVPN(dir)
	default:
		err = fmt.Errorf("unknown role %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "labd:", err)
		os.Exit(1)
	}
}

// waitForSignal blocks until the service is told to stop.
func waitForSignal() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	<-signals
}
