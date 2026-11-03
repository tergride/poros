// Package subfetch downloads a subscription and reads what the provider tells about it: traffic, expiry, title,
// links, how often to update. Response headers come first; `#key: value` lines at the top of the body fill in
// what the headers left out, as some providers put them there.
package subfetch

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MaxBody bounds a subscription: tens of thousands of servers fit, an endless stream does not.
const MaxBody = 64 << 20

// Longer header values are not metadata but noise, and a line break in one could smuggle a header.
const maxHeaderValue = 1000

// The `#key: value` scan looks only at the head of the body.
const metadataHead = 16 << 10

// maxUpdateInterval caps profile-update-interval at a year.
const maxUpdateInterval = 366 * 24 * time.Hour

// Info is what the provider said about the subscription. Zero values mean it said nothing.
type Info struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	// Total is the traffic quota in bytes; HasQuota tells a quota of 0 from no quota.
	Total    int64 `json:"total"`
	HasQuota bool  `json:"hasQuota"`
	// Expire is Unix seconds.
	Expire         int64         `json:"expire"`
	UpdateInterval time.Duration `json:"updateInterval"`
	Title          string        `json:"title"`
	WebPageURL     string        `json:"webPageUrl"`
	SupportURL     string        `json:"supportUrl"`
	Announce       string        `json:"announce"`
}

type Result struct {
	Body []byte
	Info Info
}

var (
	ErrTooLarge = fmt.Errorf("the subscription is larger than %d MiB", MaxBody>>20)
	ErrEmpty    = errors.New("the subscription is empty")
)

// Fetch downloads rawURL through client.
func Fetch(ctx context.Context, client *http.Client, rawURL, userAgent string) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Result{}, fmt.Errorf("the server answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	if err != nil {
		return Result{}, err
	}
	if len(body) > MaxBody {
		return Result{}, ErrTooLarge
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return Result{}, ErrEmpty
	}
	info := infoFromHeaders(resp.Header)
	fillFromBody(&info, body)
	return Result{Body: body, Info: info}, nil
}

func infoFromHeaders(h http.Header) Info {
	var info Info
	get := func(name string) string {
		value := strings.TrimSpace(h.Get(name))
		if len(value) >= maxHeaderValue || strings.ContainsAny(value, "\r\n") {
			return ""
		}
		return value
	}
	if value := get("Subscription-Userinfo"); value != "" {
		applyUserInfo(&info, value)
	}
	interval := get("Profile-Update-Interval")
	if interval == "" {
		interval = get("X-Profile-Update-Interval")
	}
	info.UpdateInterval = parseUpdateInterval(interval)
	info.Title = decodeValue(get("Profile-Title"))
	info.WebPageURL = get("Profile-Web-Page-Url")
	info.SupportURL = get("Support-Url")
	info.Announce = decodeValue(get("Announce"))
	return info
}

var userInfoField = regexp.MustCompile(`(?i)\b(upload|download|total|expire)\s*=\s*(\d+)`)

// applyUserInfo reads "upload=1; download=2; total=3; expire=1700000000". Some providers give expire in milliseconds.
func applyUserInfo(info *Info, value string) bool {
	matches := userInfoField.FindAllStringSubmatch(value, -1)
	for _, m := range matches {
		n, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(m[1]) {
		case "upload":
			info.Upload = n
		case "download":
			info.Download = n
		case "total":
			info.Total, info.HasQuota = n, true
		case "expire":
			if n > 1_000_000_000_000 {
				n /= 1000
			}
			info.Expire = n
		}
	}
	return len(matches) > 0
}

// parseUpdateInterval reads hours, tolerating h, d and s suffixes and quotes, between an hour and a year.
func parseUpdateInterval(raw string) time.Duration {
	s := strings.ToLower(strings.Trim(strings.TrimSpace(raw), `"'`))
	unit := time.Hour
	switch {
	case strings.HasSuffix(s, "h"):
		s = strings.TrimSuffix(s, "h")
	case strings.HasSuffix(s, "d"):
		s, unit = strings.TrimSuffix(s, "d"), 24*time.Hour
	case strings.HasSuffix(s, "s"):
		s, unit = strings.TrimSuffix(s, "s"), time.Second
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	if n > int64(maxUpdateInterval/unit) {
		return maxUpdateInterval
	}
	return max(time.Duration(n)*unit, time.Hour).Truncate(time.Hour)
}

// decodeValue reads a value that may be "base64:<text>", as providers send non-ASCII titles.
func decodeValue(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) < 7 || !strings.EqualFold(s[:7], "base64:") {
		return s
	}
	decoded, ok := decodeBase64(strings.TrimSpace(s[7:]))
	if !ok {
		return ""
	}
	return strings.TrimSpace(string(decoded))
}

// fillFromBody reads leading `#key: value` or `//key: value` lines, decoding the head first when the body is one base64 blob.
func fillFromBody(info *Info, body []byte) {
	head := bytes.TrimSpace(body)
	if len(head) > metadataHead {
		head = head[:metadataHead]
	}
	if compact := withoutSpace(head); looksLikeBase64(compact) {
		// A head cut in the middle of a quantum still decodes up to the cut.
		if decoded, ok := decodeBase64(string(compact[:len(compact)&^3])); ok {
			head = decoded
		}
	}
	userInfoSeen := info.HasQuota || info.Upload != 0 || info.Download != 0 || info.Expire != 0
	for line := range strings.Lines(string(head)) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var comment string
		switch {
		case strings.HasPrefix(line, "//"):
			comment = line[2:]
		case strings.HasPrefix(line, "#"):
			comment = line[1:]
		default:
			return
		}
		key, value, ok := strings.Cut(comment, ":")
		if !ok {
			continue
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		switch {
		case strings.HasPrefix(key, "subscription-userinfo") && !userInfoSeen:
			userInfoSeen = applyUserInfo(info, value)
		case strings.HasPrefix(key, "profile-update-interval") && info.UpdateInterval == 0:
			info.UpdateInterval = parseUpdateInterval(value)
		case strings.HasPrefix(key, "profile-title") && info.Title == "":
			info.Title = decodeValue(value)
		case strings.HasPrefix(key, "profile-web-page-url") && info.WebPageURL == "":
			info.WebPageURL = value
		case strings.HasPrefix(key, "support-url") && info.SupportURL == "":
			info.SupportURL = value
		case (strings.HasPrefix(key, "announce") || strings.HasPrefix(key, "notice")) && info.Announce == "":
			info.Announce = decodeValue(value)
		}
	}
}

func withoutSpace(b []byte) []byte {
	return bytes.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, b)
}

func looksLikeBase64(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	for _, c := range b {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '+', c == '/', c == '-', c == '_', c == '=':
		default:
			return false
		}
	}
	return true
}

// decodeBase64 accepts the standard and URL alphabets, with or without padding.
func decodeBase64(s string) ([]byte, bool) {
	s = strings.TrimRight(s, "=")
	for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
		if decoded, err := enc.DecodeString(s); err == nil {
			return decoded, true
		}
	}
	return nil, false
}
