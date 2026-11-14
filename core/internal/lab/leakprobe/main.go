//go:build windows

// Command leakprobe runs the leak probes of the lab inside poros-test: programs that are not Poros and reach the lab's
// internet every interval by every way a program can, so that the gateway sees which of them went around the VPN.
//
//	leakprobe -scenario S -out probes.jsonl -duration 40s [-interval 200ms] [-probes tcp4,udp4,…]
//
// Each attempt is a JSON line: the time, the probe, its number, whether it got through and, for the echoes, which
// address the lab's internet saw it come from ("via": user is around the VPN, vpn through it).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type record struct {
	Time  string `json:"t"`
	Probe string `json:"probe"`
	Seq   int    `json:"seq"`
	OK    bool   `json:"ok"`
	MS    int64  `json:"ms"`
	Via   string `json:"via,omitempty"`
	Src   string `json:"src,omitempty"`
	Err   string `json:"err,omitempty"`
}

type result struct {
	via, src string
}

type probe func(ctx context.Context, scenario string, seq int) (result, error)

var all = map[string]probe{
	"tcp4":       httpProbe("198.51.100.10", "tcp4"),
	"tcp6":       httpProbe("[3fff:0:100::10]", "tcp6"),
	"udp4":       udpProbe("198.51.100.10:7777"),
	"udp6":       udpProbe("[3fff:0:100::10]:7777"),
	"dns":        systemDNSProbe,
	"dnsdirect4": directDNSProbe("8.8.8.8:53", "udp"),
	"dnsdirect6": directDNSProbe("[2001:4860:4860::8888]:53", "udp"),
	"dnstcp":     directDNSProbe("1.1.1.1:53", "tcp"),
	"dot":        directDNSProbe("1.1.1.1:853", "tcp-tls"),
	"bound4":     boundProbe("198.51.100.12", "tcp4"),
	"bound6":     boundProbe("3fff:0:100::12", "tcp6"),
	"sysproxy":   systemProxyProbe,
	"stun4":      stunProbe("198.51.100.3:3478"),
	"stun6":      stunProbe("[3fff:0:100::3]:3478"),
	"icmp4":      icmpProbe("198.51.100.13"),
	"icmp6":      icmpProbe("3fff:0:100::13"),
	"long4":      longProbe("198.51.100.10:7777"),
	"long6":      longProbe("[3fff:0:100::10]:7777"),
}

// slow probes start a process or wait long: they run once a second at most.
var slow = map[string]bool{"icmp4": true, "icmp6": true}

func main() {
	scenario := flag.String("scenario", "s", "the scenario's name, carried in every request")
	out := flag.String("out", "probes.jsonl", "where the records go")
	duration := flag.Duration("duration", 40*time.Second, "how long to probe")
	interval := flag.Duration("interval", 200*time.Millisecond, "how often each probe tries")
	only := flag.String("probes", "", "the probes to run, comma-separated; all by default")
	flag.Parse()

	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	if *only != "" {
		names = strings.Split(*only, ",")
	}
	sort.Strings(names)
	file, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer file.Close()
	var mu sync.Mutex
	write := func(r record) {
		line, _ := json.Marshal(r)
		mu.Lock()
		defer mu.Unlock()
		_, _ = file.Write(append(line, '\n'))
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()
	var wg sync.WaitGroup
	for _, name := range names {
		run, ok := all[name]
		if !ok {
			fmt.Fprintln(os.Stderr, "no probe", name)
			os.Exit(2)
		}
		every := *interval
		if slow[name] {
			every = max(every, time.Second)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(every)
			defer ticker.Stop()
			for seq := 1; ; seq++ {
				started := time.Now()
				attempt, cancelAttempt := context.WithTimeout(ctx, max(every, time.Second))
				got, err := run(attempt, *scenario, seq)
				cancelAttempt()
				r := record{Time: started.UTC().Format(time.RFC3339Nano), Probe: name, Seq: seq, OK: err == nil,
					MS: time.Since(started).Milliseconds(), Via: got.via, Src: got.src}
				if err != nil {
					r.Err = err.Error()
				}
				if ctx.Err() == nil {
					write(r)
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
	wg.Wait()
}
