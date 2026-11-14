package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// The ports of the lab's internet: TCP services answer on every address of its namespace, UDP ones on these.
const (
	echoPort = 7777
	stunPort = 3478
)

var (
	resolverAddresses = []string{"198.51.100.53", "1.1.1.1", "8.8.8.8", "9.9.9.9", "3fff:0:100::53",
		"2606:4700:4700::1111", "2001:4860:4860::8888"}
	stunAddresses = []string{"198.51.100.3", "3fff:0:100::3"}
	echoAddresses = []string{"198.51.100.10", "198.51.100.11", "3fff:0:100::10", "3fff:0:100::11"}
)

type inet struct {
	dir  string
	log  *eventLog
	ca   *authority
	leak leakTests
}

func runInet(dir string) error {
	ca, err := loadCA(dir)
	if err != nil {
		return err
	}
	log, err := openEventLog(filepath.Join(dir, "inet.jsonl"))
	if err != nil {
		return err
	}
	n := &inet{dir: dir, log: log, ca: ca, leak: leakTests{resolvers: map[string]map[string]bool{}}}
	errs := make(chan error, 8)
	serve := func(name string, run func() error) {
		go func() { errs <- fmt.Errorf("%s: %w", name, run()) }()
	}

	handler := http.HandlerFunc(n.serveHTTP)
	plain := &http.Server{Addr: ":80", Handler: handler, ConnContext: withConn}
	secure := &http.Server{Addr: ":443", Handler: handler, ConnContext: withConn, TLSConfig: ca.tlsConfig()}
	serve("http", plain.ListenAndServe)
	serve("https", func() error { return secure.ListenAndServeTLS("", "") })

	dnsHandler := dns.HandlerFunc(n.serveDNS)
	tcpDNS := &dns.Server{Addr: ":53", Net: "tcp", Handler: dnsHandler}
	serve("dns/tcp", tcpDNS.ListenAndServe)
	// UDP answers from the address it was asked at only when the socket is bound to it: the namespace takes whole
	// ranges as its own, and a socket on all of them would answer from another address of its choice.
	for _, address := range resolverAddresses {
		server := &dns.Server{Addr: net.JoinHostPort(address, "53"), Net: "udp", Handler: dnsHandler}
		serve("dns/udp "+address, server.ListenAndServe)
	}
	dot := &dns.Server{Addr: ":853", Net: "tcp-tls", Handler: dnsHandler, TLSConfig: ca.tlsConfig()}
	serve("dot", dot.ListenAndServe)

	for _, address := range stunAddresses {
		serve("stun "+address, func() error { return n.serveSTUN(address) })
	}
	for _, address := range echoAddresses {
		serve("udp echo "+address, func() error { return n.serveUDPEcho(address) })
	}
	serve("tcp echo", n.serveTCPEcho)

	fmt.Fprintln(os.Stderr, "labd inet: ready")
	go func() { waitForSignal(); errs <- nil }()
	return <-errs
}

type connKey struct{}

// withConn keeps the connection in the request's context: the address it came to tells which service was asked.
func withConn(ctx context.Context, conn net.Conn) context.Context {
	return context.WithValue(ctx, connKey{}, conn)
}

func (n *inet) serveHTTP(w http.ResponseWriter, r *http.Request) {
	local := ""
	if conn, ok := r.Context().Value(connKey{}).(net.Conn); ok {
		local = conn.LocalAddr().String()
	}
	host := strings.ToLower(r.Host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	kind := "http"
	if r.TLS != nil {
		kind = "https"
	}
	n.log.add(event{Kind: kind, Src: r.RemoteAddr, Dst: local, Name: host, Path: r.URL.RequestURI()})
	src := hostOf(r.RemoteAddr)
	where := placeOf(r.RemoteAddr)

	switch {
	case r.URL.Path == "/dns-query" || r.URL.Path == "/resolve":
		n.serveDoH(w, r)
	case host == "ipleak.net":
		writeJSON(w, map[string]any{"ip": src, "country_code": where.Country, "city_name": where.City,
			"isp_name": where.ISP, "as_number": where.ASN})
	case host == "ipinfo.io":
		writeJSON(w, map[string]any{"ip": src, "country": where.Country, "city": where.City,
			"org": fmt.Sprintf("AS%d %s", where.ASN, where.ISP)})
	case host == "ipwho.is":
		writeJSON(w, map[string]any{"ip": src, "success": true, "country_code": where.Country, "city": where.City,
			"connection": map[string]any{"asn": where.ASN, "isp": where.ISP}})
	case host == "api.ip2location.io":
		writeJSON(w, map[string]any{"ip": src, "country_code": where.Country})
	case host == "bash.ws":
		n.serveLeakTest(w, r)
	case host == "cp.cloudflare.com":
		w.WriteHeader(http.StatusNoContent)
	case host == "speed.cloudflare.com":
		serveSpeed(w, r)
	case host == "www.msftconnecttest.com" || host == "ipv6.msftconnecttest.com":
		_, _ = io.WriteString(w, "Microsoft Connect Test")
	case host == "sub.lab":
		sub, err := os.ReadFile(filepath.Join(n.dir, "sub.txt"))
		if err != nil {
			http.Error(w, "the VPN servers are not up yet", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(sub)
	case host == "github.com":
		http.NotFound(w, r)
	default:
		// A probe: it learns which address the internet saw it come from.
		writeJSON(w, map[string]any{"ip": src, "via": viaOf(r.RemoteAddr), "local": local, "host": host,
			"path": r.URL.Path, "proto": r.Proto})
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

// serveSpeed stands in for the speed test: __down sends as many bytes as asked, __up takes what comes.
func serveSpeed(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/__down":
		size, _ := strconv.ParseInt(r.URL.Query().Get("bytes"), 10, 64)
		size = min(max(size, 0), 256<<20)
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		chunk := make([]byte, 64<<10)
		for size > 0 {
			part := min(size, int64(len(chunk)))
			if _, err := w.Write(chunk[:part]); err != nil {
				return
			}
			size -= part
		}
	case "/__up":
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 256<<20))
	default:
		http.NotFound(w, r)
	}
}

// serveDoH answers DNS over HTTPS in both forms: the wire form of RFC 8484 that sing-box speaks, and the JSON form
// that Poros asks 1.1.1.1 and 8.8.8.8 in to find its servers.
func (n *inet) serveDoH(w http.ResponseWriter, r *http.Request) {
	local := ""
	if conn, ok := r.Context().Value(connKey{}).(net.Conn); ok {
		local = conn.LocalAddr().String()
	}
	var wire []byte
	switch {
	case r.Method == http.MethodPost:
		wire, _ = io.ReadAll(io.LimitReader(r.Body, 64<<10))
	case r.URL.Query().Get("dns") != "":
		wire, _ = base64.RawURLEncoding.DecodeString(strings.TrimRight(r.URL.Query().Get("dns"), "="))
	}
	if wire != nil {
		query := new(dns.Msg)
		if err := query.Unpack(wire); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		packed, err := n.answer(query, "doh", r.RemoteAddr, local).Pack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(packed)
		return
	}
	name := r.URL.Query().Get("name")
	kind, _ := strconv.Atoi(r.URL.Query().Get("type"))
	if kind == 0 {
		kind = int(dns.TypeA)
	}
	n.log.add(event{Kind: "doh", Src: r.RemoteAddr, Name: name, Detail: dns.TypeToString[uint16(kind)]})
	type answer struct {
		Name string `json:"name"`
		Type int    `json:"type"`
		TTL  int    `json:"TTL"`
		Data string `json:"data"`
	}
	reply := struct {
		Status int      `json:"Status"`
		Answer []answer `json:"Answer,omitempty"`
	}{}
	addrs := lookup(name)
	if addrs == nil {
		reply.Status = dns.RcodeNameError
	}
	for _, addr := range addrs {
		if (kind == int(dns.TypeA)) == addr.Is4() {
			reply.Answer = append(reply.Answer, answer{Name: dns.Fqdn(name), Type: kind, TTL: 60, Data: addr.String()})
		}
	}
	w.Header().Set("Content-Type", "application/dns-json")
	_ = json.NewEncoder(w).Encode(reply)
}

func (n *inet) serveDNS(w dns.ResponseWriter, r *dns.Msg) {
	_ = w.WriteMsg(n.answer(r, "dns", w.RemoteAddr().String(), w.LocalAddr().String()))
}

// answer resolves a query from the lab's names, and logs who asked whom.
func (n *inet) answer(r *dns.Msg, kind, src, dst string) *dns.Msg {
	reply := new(dns.Msg)
	reply.SetReply(r)
	reply.Authoritative = true
	for _, question := range r.Question {
		n.log.add(event{Kind: kind, Src: src, Dst: dst, Name: question.Name, Detail: dns.TypeToString[question.Qtype]})
		n.leak.saw(question.Name, src)
		addrs := lookup(question.Name)
		if addrs == nil {
			reply.Rcode = dns.RcodeNameError
			continue
		}
		for _, addr := range addrs {
			header := dns.RR_Header{Name: question.Name, Class: dns.ClassINET, Ttl: 60}
			switch {
			case question.Qtype == dns.TypeA && addr.Is4():
				header.Rrtype = dns.TypeA
				reply.Answer = append(reply.Answer, &dns.A{Hdr: header, A: addr.AsSlice()})
			case question.Qtype == dns.TypeAAAA && addr.Is6():
				header.Rrtype = dns.TypeAAAA
				reply.Answer = append(reply.Answer, &dns.AAAA{Hdr: header, AAAA: addr.AsSlice()})
			}
		}
	}
	return reply
}

// leakTests stands in for bash.ws: it hands out test IDs and remembers which resolvers asked for names under them.
type leakTests struct {
	mu        sync.Mutex
	resolvers map[string]map[string]bool
}

var leakName = regexp.MustCompile(`^\d+\.([0-9a-z]+)\.bash\.ws\.?$`)

func (l *leakTests) saw(name, src string) {
	match := leakName.FindStringSubmatch(strings.ToLower(name))
	if match == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if seen, ok := l.resolvers[match[1]]; ok {
		seen[hostOf(src)] = true
	}
}

func (n *inet) serveLeakTest(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/id":
		id, _ := rand.Int(rand.Reader, big.NewInt(1e9))
		text := strconv.FormatInt(id.Int64()+1e9, 10)
		n.leak.mu.Lock()
		n.leak.resolvers[text] = map[string]bool{}
		n.leak.mu.Unlock()
		_, _ = io.WriteString(w, text)
	case strings.HasPrefix(r.URL.Path, "/dnsleak/test/"):
		id := strings.TrimPrefix(r.URL.Path, "/dnsleak/test/")
		n.leak.mu.Lock()
		seen := n.leak.resolvers[id]
		var resolvers []string
		for ip := range seen {
			resolvers = append(resolvers, ip)
		}
		n.leak.mu.Unlock()
		where := placeOf(r.RemoteAddr)
		records := []map[string]any{{"type": "ip", "ip": hostOf(r.RemoteAddr), "country": where.Country,
			"city_name": where.City, "asn": fmt.Sprintf("AS%d %s", where.ASN, where.ISP)}}
		for _, ip := range resolvers {
			at := placeOf(ip)
			records = append(records, map[string]any{"type": "dns", "ip": ip, "country": at.Country,
				"city_name": at.City, "asn": fmt.Sprintf("AS%d %s", at.ASN, at.ISP)})
		}
		writeJSON(w, records)
	default:
		http.NotFound(w, r)
	}
}

// serveSTUN answers binding requests (RFC 5389) with the address the request came from, as WebRTC asks.
func (n *inet) serveSTUN(address string) error {
	conn, err := net.ListenPacket("udp", net.JoinHostPort(address, strconv.Itoa(stunPort)))
	if err != nil {
		return err
	}
	const cookie = 0x2112A442
	buffer := make([]byte, 1500)
	for {
		size, from, err := conn.ReadFrom(buffer)
		if err != nil {
			return err
		}
		packet := buffer[:size]
		if size < 20 || binary.BigEndian.Uint16(packet) != 0x0001 || binary.BigEndian.Uint32(packet[4:]) != cookie {
			continue
		}
		n.log.add(event{Kind: "stun", Src: from.String(), Dst: conn.LocalAddr().String()})
		peer := from.(*net.UDPAddr)
		ip := peer.IP.To4()
		family := byte(1)
		if ip == nil {
			ip, family = peer.IP.To16(), 2
		}
		xored := make([]byte, len(ip))
		mask := append(binary.BigEndian.AppendUint32(nil, cookie), packet[8:20]...)
		for i := range ip {
			xored[i] = ip[i] ^ mask[i]
		}
		attribute := append([]byte{0, family}, binary.BigEndian.AppendUint16(nil, uint16(peer.Port)^uint16(cookie>>16))...)
		attribute = append(attribute, xored...)
		reply := binary.BigEndian.AppendUint16(nil, 0x0101)
		reply = binary.BigEndian.AppendUint16(reply, uint16(4+len(attribute)))
		reply = append(reply, packet[4:20]...)
		reply = binary.BigEndian.AppendUint16(reply, 0x0020)
		reply = binary.BigEndian.AppendUint16(reply, uint16(len(attribute)))
		reply = append(reply, attribute...)
		_, _ = conn.WriteTo(reply, from)
	}
}

// serveUDPEcho sends each datagram back after the address it came from.
func (n *inet) serveUDPEcho(address string) error {
	conn, err := net.ListenPacket("udp", net.JoinHostPort(address, strconv.Itoa(echoPort)))
	if err != nil {
		return err
	}
	buffer := make([]byte, 65536)
	for {
		size, from, err := conn.ReadFrom(buffer)
		if err != nil {
			return err
		}
		payload := string(buffer[:min(size, 120)])
		n.log.add(event{Kind: "udp", Src: from.String(), Detail: printable(payload)})
		_, _ = conn.WriteTo([]byte("src="+from.String()+" via="+viaOf(from.String())+" "+payload), from)
	}
}

// serveTCPEcho answers each line of a connection with the address it came from: a connection that stays open shows
// whether it goes on along the same way after a change.
func (n *inet) serveTCPEcho() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", echoPort))
	if err != nil {
		return err
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer conn.Close()
			reader := bufio.NewReader(io.LimitReader(conn, 1<<20))
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				line = strings.TrimSpace(line)
				n.log.add(event{Kind: "tcp", Src: conn.RemoteAddr().String(), Dst: conn.LocalAddr().String(), Detail: printable(line)})
				if _, err = fmt.Fprintf(conn, "src=%s via=%s %s\n", conn.RemoteAddr(), viaOf(conn.RemoteAddr().String()), line); err != nil {
					return
				}
			}
		}()
	}
}

func printable(text string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r > 126 {
			return '.'
		}
		return r
	}, text)
}
