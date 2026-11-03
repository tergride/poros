package profile

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// parseHysteria2Link reads hysteria2://password@host:port/?query#name, also as hy2://. The port may be a port
// hopping list such as 20000-30000 or 443,5000-6000, and mport may carry one too.
func parseHysteria2Link(link string) (Profile, error) {
	l, err := parseShareLink(link)
	if err != nil {
		return Profile{}, err
	}
	q := l.query
	// The official URI leaves the password unescaped, so a ':' in it belongs to it.
	h := &Hysteria2{Password: l.userinfo()}

	var port int
	if strings.ContainsAny(l.port, ",-") {
		if h.ServerPorts, err = hysteriaPortRanges(l.port); err != nil {
			return Profile{}, err
		}
	} else if port, err = l.serverPort(0); err != nil {
		return Profile{}, err
	}
	if q.has("mport") {
		if h.ServerPorts, err = hysteriaPortRanges(q.get("mport")); err != nil {
			return Profile{}, err
		}
	}
	if port == 0 && len(h.ServerPorts) > 0 {
		first, _, _ := strings.Cut(h.ServerPorts[0], ":")
		port, _ = strconv.Atoi(first)
	}
	if port == 0 {
		port = 443
	}

	if password := q.get("obfs-password"); password != "" {
		obfs := strings.ToLower(q.trimmed("obfs"))
		// The Qt client read packet sizes without obfs as gecko, which only its patched core had.
		if !q.has("obfs") && (q.has("min_packet_size") || q.has("max_packet_size")) {
			obfs = "gecko"
		}
		if obfs != "" && obfs != "salamander" {
			return Profile{}, fmt.Errorf("the Hysteria2 obfuscation %q is not supported", obfs)
		}
		h.ObfsType, h.ObfsPassword = "salamander", password
	}
	if q.has("upmbps") {
		h.UpMbps = hysteriaLinkMbps(q.get("upmbps"))
	}
	if q.has("downmbps") {
		h.DownMbps = hysteriaLinkMbps(q.get("downmbps"))
	}
	h.HopInterval = linkDuration(q, "hop_interval")

	tls, err := linkTLS(q, true)
	if err != nil {
		return Profile{}, err
	}
	h.TLS = quicLinkTLS(tls)
	return Profile{Type: TypeHysteria2, Name: l.name, Server: l.host, ServerPort: port, Hysteria2: h}, nil
}

// hysteriaPortRanges turns "443,20000-30000" into the "443:443" and "20000:30000" ranges sing-box takes.
func hysteriaPortRanges(text string) ([]string, error) {
	var ranges []string
	for item := range strings.SplitSeq(text, ",") {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		start, end, isRange := strings.Cut(item, "-")
		if !isRange {
			start, end, isRange = strings.Cut(item, ":")
		}
		if !isRange {
			end = start
		}
		first, firstOK := linkPortNumber(start)
		last, lastOK := linkPortNumber(end)
		if !firstOK || !lastOK || first == 0 || last == 0 {
			return nil, fmt.Errorf("the port range %q is not from 1 to 65535", item)
		}
		ranges = append(ranges, strconv.Itoa(first)+":"+strconv.Itoa(last))
	}
	return ranges, nil
}

// A bare number is Mbps; only the final b or B tells bits from bytes, so "100 Mbps" is 100 and "50 MB/s" is 400.
var hysteriaBandwidthPattern = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*([KMGTkmgt]?)(?:([bB])(?:ps|/s)?|ps)?$`)

// hysteriaLinkMbps reads upmbps and downmbps: the URI says a bare number, but panels commonly write "55Mbps".
func hysteriaLinkMbps(text string) int {
	match := hysteriaBandwidthPattern.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil {
		return 0
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil || math.IsInf(value, 0) {
		return 0
	}
	mbps := value
	if prefix, unit := strings.ToUpper(match[2]), match[3]; prefix != "" || unit != "" {
		scale := map[string]float64{"": 1e-6, "K": 1e-3, "M": 1, "G": 1e3, "T": 1e6}[prefix]
		if unit == "B" {
			scale *= 8
		}
		mbps = value * scale
	}
	if mbps <= 0 {
		return 0
	}
	return max(1, int(math.Round(min(mbps, math.MaxInt32))))
}
