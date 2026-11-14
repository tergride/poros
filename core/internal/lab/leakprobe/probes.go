//go:build windows

package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/sys/windows/registry"
)

// echo is what the lab's sites answer: the address they saw and what it stands for.
type echo struct {
	IP  string `json:"ip"`
	Via string `json:"via"`
}

func probePath(scenario, name string, seq int) string {
	return fmt.Sprintf("/p/%s/%s/%d", url.PathEscape(scenario), name, seq)
}

// httpProbe asks the lab's echo site without a proxy, as a program that knows none: with TUN it must go through the
// VPN, without TUN it goes around it by design.
func httpProbe(host, network string) probe {
	dialer := &net.Dialer{}
	client := &http.Client{Transport: &http.Transport{
		Proxy:             nil,
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, address)
		},
	}}
	return func(ctx context.Context, scenario string, seq int) (result, error) {
		return getEcho(ctx, client, "http://"+host+probePath(scenario, network, seq))
	}
}

func getEcho(ctx context.Context, client *http.Client, address string) (result, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return result{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return result{}, err
	}
	defer response.Body.Close()
	var answer echo
	if err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&answer); err != nil {
		return result{}, fmt.Errorf("status %s: %w", response.Status, err)
	}
	return result{via: answer.Via, src: answer.IP}, nil
}

// systemProxyProbe asks through the proxy of Windows, as browsers do; with no proxy set it goes directly.
func systemProxyProbe(ctx context.Context, scenario string, seq int) (result, error) {
	transport := &http.Transport{DisableKeepAlives: true}
	if proxy := windowsProxy(); proxy != "" {
		address, err := url.Parse("http://" + proxy)
		if err != nil {
			return result{}, err
		}
		transport.Proxy = http.ProxyURL(address)
	}
	client := &http.Client{Transport: transport}
	return getEcho(ctx, client, "http://echo.lab"+probePath(scenario, "sysproxy", seq))
}

// windowsProxy reads the proxy the user's Internet settings point at, "" when it is off.
func windowsProxy() string {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return ""
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return ""
	}
	// "http=host:port;https=…" or a single "host:port".
	for _, part := range strings.Split(server, ";") {
		if value, ok := strings.CutPrefix(part, "http="); ok {
			return value
		}
	}
	return strings.Split(server, ";")[0]
}

func udpProbe(address string) probe {
	return func(ctx context.Context, scenario string, seq int) (result, error) {
		var dialer net.Dialer
		conn, err := dialer.DialContext(ctx, "udp", address)
		if err != nil {
			return result{}, err
		}
		defer conn.Close()
		if deadline, ok := ctx.Deadline(); ok {
			_ = conn.SetDeadline(deadline)
		}
		if _, err = fmt.Fprintf(conn, "%s/udp/%d", scenario, seq); err != nil {
			return result{}, err
		}
		reply := make([]byte, 512)
		size, err := conn.Read(reply)
		if err != nil {
			return result{}, err
		}
		return parseEchoLine(string(reply[:size]))
	}
}

// parseEchoLine reads "src=<address> via=<what> …" of the lab's UDP and TCP echoes.
func parseEchoLine(line string) (result, error) {
	var r result
	for _, field := range strings.Fields(line) {
		if value, ok := strings.CutPrefix(field, "src="); ok {
			r.src = value
		}
		if value, ok := strings.CutPrefix(field, "via="); ok {
			r.via = value
		}
	}
	if r.via == "" {
		return r, fmt.Errorf("not an echo: %q", line)
	}
	return r, nil
}

// systemDNSProbe resolves a fresh name through Windows, as any program does: the gateway's logs tell who asked.
func systemDNSProbe(ctx context.Context, scenario string, seq int) (result, error) {
	name := fmt.Sprintf("%d-dns.%s.probe.lab", seq, dnsLabel(scenario))
	addrs, err := net.DefaultResolver.LookupHost(ctx, name)
	if err != nil {
		return result{}, err
	}
	return result{src: strings.Join(addrs, " ")}, nil
}

// directDNSProbe asks a resolver of the lab's internet itself, as programs with a DNS of their own do.
func directDNSProbe(server, network string) probe {
	return func(ctx context.Context, scenario string, seq int) (result, error) {
		client := &dns.Client{Net: network, Timeout: time.Second}
		message := new(dns.Msg)
		message.SetQuestion(fmt.Sprintf("%d-%s.%s.probe.lab.", seq, strings.ReplaceAll(network, "-", ""), dnsLabel(scenario)), dns.TypeA)
		reply, _, err := client.ExchangeContext(ctx, message, server)
		if err != nil {
			return result{}, err
		}
		if len(reply.Answer) == 0 {
			return result{}, fmt.Errorf("no answer, rcode %s", dns.RcodeToString[reply.Rcode])
		}
		return result{src: reply.Answer[0].String()}, nil
	}
}

// dnsLabel makes a scenario name fit one DNS label.
func dnsLabel(scenario string) string {
	label := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return '-'
	}, scenario)
	return label[:min(len(label), 50)]
}

// boundProbe dials from the lab adapter itself, past the routing table, as some games and P2P programs do: only a
// firewall block can stop it.
func boundProbe(host, network string) probe {
	return func(ctx context.Context, scenario string, seq int) (result, error) {
		index, err := labAdapter()
		if err != nil {
			return result{}, err
		}
		dialer := &net.Dialer{Control: func(_, _ string, raw syscall.RawConn) error {
			var setErr error
			err := raw.Control(func(fd uintptr) { setErr = bindToInterface(fd, network, index) })
			return errors.Join(err, setErr)
		}}
		client := &http.Client{Transport: &http.Transport{
			Proxy:             nil,
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, address)
			},
		}}
		target := host
		if strings.Contains(host, ":") {
			target = "[" + host + "]"
		}
		return getEcho(ctx, client, "http://"+target+probePath(scenario, "bound"+network[3:], seq))
	}
}

// labAdapter is the index of the adapter with an address of the lab: 192.168.77.0/24 or 192.168.78.0/24.
func labAdapter() (int, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return 0, err
	}
	lab := []netip.Prefix{netip.MustParsePrefix("192.168.77.0/24"), netip.MustParsePrefix("192.168.78.0/24")}
	for _, iface := range interfaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			prefix, err := netip.ParsePrefix(addr.String())
			if err != nil {
				continue
			}
			for _, network := range lab {
				if network.Contains(prefix.Addr()) && iface.Flags&net.FlagUp != 0 {
					return iface.Index, nil
				}
			}
		}
	}
	return 0, errors.New("no adapter on the lab network")
}

// stunProbe asks a STUN server for the address it sees, as WebRTC does in a browser.
func stunProbe(address string) probe {
	return func(ctx context.Context, scenario string, seq int) (result, error) {
		var dialer net.Dialer
		conn, err := dialer.DialContext(ctx, "udp", address)
		if err != nil {
			return result{}, err
		}
		defer conn.Close()
		if deadline, ok := ctx.Deadline(); ok {
			_ = conn.SetDeadline(deadline)
		}
		request := make([]byte, 20)
		binary.BigEndian.PutUint16(request, 0x0001)
		binary.BigEndian.PutUint32(request[4:], 0x2112A442)
		_, _ = rand.Read(request[8:])
		if _, err = conn.Write(request); err != nil {
			return result{}, err
		}
		reply := make([]byte, 512)
		size, err := conn.Read(reply)
		if err != nil {
			return result{}, err
		}
		mapped, err := xorMapped(reply[:size], request[4:20])
		if err != nil {
			return result{}, err
		}
		return result{src: mapped.String(), via: viaOfMapped(mapped.Addr())}, nil
	}
}

func xorMapped(reply, mask []byte) (netip.AddrPort, error) {
	if len(reply) < 20 || binary.BigEndian.Uint16(reply) != 0x0101 {
		return netip.AddrPort{}, errors.New("not a STUN binding response")
	}
	for at := 20; at+4 <= len(reply); {
		kind := binary.BigEndian.Uint16(reply[at:])
		size := int(binary.BigEndian.Uint16(reply[at+2:]))
		value := reply[at+4 : min(at+4+size, len(reply))]
		if kind == 0x0020 && len(value) >= 8 {
			port := binary.BigEndian.Uint16(value[2:]) ^ 0x2112
			raw := make([]byte, len(value)-4)
			for i := range raw {
				raw[i] = value[4+i] ^ mask[i]
			}
			addr, ok := netip.AddrFromSlice(raw)
			if !ok {
				return netip.AddrPort{}, errors.New("a bad mapped address")
			}
			return netip.AddrPortFrom(addr.Unmap(), port), nil
		}
		at += 4 + (size+3)&^3
	}
	return netip.AddrPort{}, errors.New("no mapped address")
}

// viaOfMapped tells the lab's addresses apart the way the lab's internet does.
func viaOfMapped(addr netip.Addr) string {
	switch {
	case addr == netip.MustParseAddr("192.0.2.77"), netip.MustParsePrefix("3fff:0:77::/48").Contains(addr),
		netip.MustParsePrefix("3fff:0:78::/48").Contains(addr):
		return "user"
	case netip.MustParsePrefix("203.0.113.0/24").Contains(addr), netip.MustParsePrefix("3fff:0:203::/48").Contains(addr):
		return "vpn"
	}
	return "other"
}

// longProbe keeps one connection to the lab's TCP echo and sends a line on it each time: a connection opened before
// a change, such as the start of the VPN, shows whether it goes on around the VPN after it. A broken connection is
// opened again on the next try.
func longProbe(address string) probe {
	var conn net.Conn
	var reader *bufio.Reader
	return func(ctx context.Context, scenario string, seq int) (result, error) {
		if conn == nil {
			var dialer net.Dialer
			c, err := dialer.DialContext(ctx, "tcp", address)
			if err != nil {
				return result{}, err
			}
			conn, reader = c, bufio.NewReader(c)
		}
		fail := func(err error) (result, error) {
			conn.Close()
			conn = nil
			return result{}, err
		}
		if deadline, ok := ctx.Deadline(); ok {
			_ = conn.SetDeadline(deadline)
		}
		if _, err := fmt.Fprintf(conn, "%s/long/%d\n", scenario, seq); err != nil {
			return fail(err)
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			return fail(err)
		}
		return parseEchoLine(line)
	}
}

// icmpProbe pings once with the ping of Windows, which exits with 0 only on a reply. Whether the reply came through
// the VPN is the gateway's to tell: ICMP carries no answer of the lab.
func icmpProbe(host string) probe {
	return func(ctx context.Context, scenario string, seq int) (result, error) {
		command := exec.CommandContext(ctx, "ping", "-n", "1", "-w", "800", host)
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if err := command.Run(); err != nil {
			return result{}, fmt.Errorf("no reply")
		}
		return result{}, nil
	}
}
