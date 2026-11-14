package otp

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Parse reads codes from text: JSON of Poros as a whole, else one otpauth:// or otpauth-migration:// link or bare
// secret per line. What cannot be taken is told by its line and never repeated with its secret.
func Parse(text string) ([]Entry, []Problem) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		return parseJSON(text)
	}
	var entries []Entry
	var problems []Problem
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "otpauth-migration://"):
			batch, skipped := parseMigration(line)
			entries = append(entries, batch...)
			for _, p := range skipped {
				p.Line = i + 1
				problems = append(problems, p)
			}
		case strings.HasPrefix(lower, "otpauth://"):
			entry, problem := ParseLink(line)
			if problem != nil {
				problem.Line = i + 1
				problems = append(problems, *problem)
				continue
			}
			entries = append(entries, entry)
		case looksLikeSecret(line):
			key, ok := decodeBase32(line)
			switch {
			case !ok:
				problems = append(problems, Problem{Line: i + 1, Reason: ReasonBadSecret})
			case len(key) < minBareSecret:
				problems = append(problems, Problem{Line: i + 1, Reason: ReasonShortSecret})
			default:
				entries = append(entries, New(encodeBase32(key)))
			}
		default:
			problems = append(problems, Problem{Line: i + 1, Reason: ReasonUnknown})
		}
	}
	return entries, problems
}

var secretGroups = regexp.MustCompile(`[\s-]+`)

// looksLikeSecret tells a printed secret from prose: one word, or groups of the same width.
func looksLikeSecret(line string) bool {
	groups := strings.Fields(secretGroups.ReplaceAllString(line, " "))
	if len(groups) == 1 {
		return true
	}
	if len(groups) == 0 || len(groups[0]) > 8 {
		return false
	}
	for _, g := range groups {
		if len(g) != len(groups[0]) {
			return false
		}
	}
	return true
}

// ParseLink reads an otpauth:// link. The label is "Name:Account" or one of them; the issuer parameter names the
// service over the label's.
func ParseLink(link string) (Entry, *Problem) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || !strings.EqualFold(u.Scheme, "otpauth") {
		return Entry{}, &Problem{Reason: ReasonBadLink}
	}
	entry := New("")
	label := strings.TrimSpace(strings.TrimPrefix(u.Path, "/"))
	query := u.Query()
	issuer := strings.TrimSpace(query.Get("issuer"))
	switch {
	// A label that starts with the issuer keeps a colon of the name: "Corp:VPN:alice" with the issuer "Corp:VPN".
	case issuer != "" && label == issuer:
		entry.Name = issuer
	case issuer != "" && strings.HasPrefix(label, issuer+":"):
		entry.Name, entry.Account = issuer, strings.TrimSpace(label[len(issuer)+1:])
	default:
		if name, account, found := strings.Cut(label, ":"); found {
			entry.Name, entry.Account = strings.TrimSpace(name), strings.TrimSpace(account)
		} else {
			entry.Account = label
		}
		if issuer != "" {
			entry.Name = issuer
		}
	}
	if entry.Name == "" {
		entry.Name, entry.Account = entry.Account, ""
	}
	fail := func(reason, value string) (Entry, *Problem) {
		return Entry{}, &Problem{Name: entry.Name, Reason: reason, Value: value}
	}
	entry.Kind = strings.ToLower(u.Host)
	if entry.Kind != KindTOTP && entry.Kind != KindHOTP {
		return fail(ReasonBadKind, u.Host)
	}
	secret := query.Get("secret")
	if strings.TrimSpace(secret) == "" {
		return fail(ReasonNoSecret, "")
	}
	var ok bool
	if entry.Secret, ok = Normalize(secret); !ok {
		return fail(ReasonBadSecret, "")
	}
	if query.Has("algorithm") {
		value := query.Get("algorithm")
		if entry.Algorithm, ok = algorithmOf(value); !ok {
			return fail(ReasonBadAlgorithm, value)
		}
	}
	if query.Has("digits") {
		value := strings.TrimSpace(query.Get("digits"))
		digits, err := strconv.Atoi(value)
		if err != nil || digits < MinDigits || digits > MaxDigits {
			return fail(ReasonBadDigits, value)
		}
		entry.Digits = digits
	}
	// The period is TOTP's alone and the counter HOTP's: the other one is ignored.
	if entry.Kind == KindTOTP && query.Has("period") {
		value := strings.TrimSpace(query.Get("period"))
		period, err := strconv.Atoi(value)
		if err != nil || period < MinPeriod || period > MaxPeriod {
			return fail(ReasonBadPeriod, value)
		}
		entry.Period = period
	}
	if entry.Kind == KindHOTP {
		entry.Period = 0
		if query.Has("counter") {
			value := strings.TrimSpace(query.Get("counter"))
			counter, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return fail(ReasonBadCounter, value)
			}
			entry.Counter = counter
		}
	}
	return entry, nil
}

func algorithmOf(value string) (string, bool) {
	switch strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(value)), "-", "") {
	case "", SHA1:
		return SHA1, true
	case SHA256:
		return SHA256, true
	case SHA512:
		return SHA512, true
	}
	return "", false
}

// parseMigration reads Google Authenticator's export: base64 of a protobuf MigrationPayload, whose field 1 repeats
// OtpParameters. An unreadable payload is one problem; an entry Poros cannot count is skipped with its own.
func parseMigration(link string) ([]Entry, []Problem) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, []Problem{{Reason: ReasonBadLink}}
	}
	// A link pasted from elsewhere may carry the plus signs of base64 unescaped, which a query reads as spaces.
	data := strings.ReplaceAll(u.Query().Get("data"), " ", "+")
	var payload []byte
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if payload, err = encoding.DecodeString(data); err == nil {
			break
		}
	}
	if err != nil || len(payload) == 0 {
		return nil, []Problem{{Reason: ReasonBadLink}}
	}
	var entries []Entry
	var problems []Problem
	readable := forEachField(payload, func(field int, _ uint64, bytes []byte) bool {
		if field != 1 || bytes == nil {
			return true
		}
		entry, problem, ok := migrationEntry(bytes)
		switch {
		case !ok:
			return false
		case problem != nil:
			problems = append(problems, *problem)
		default:
			entries = append(entries, entry)
		}
		return true
	})
	if !readable {
		return nil, []Problem{{Reason: ReasonBadLink}}
	}
	return entries, problems
}

// migrationEntry reads one OtpParameters; false when its protobuf is broken.
func migrationEntry(data []byte) (Entry, *Problem, bool) {
	entry := New("")
	var key []byte
	var problem *Problem
	bad := func(reason, value string) {
		if problem == nil {
			problem = &Problem{Reason: reason, Value: value}
		}
	}
	ok := forEachField(data, func(field int, value uint64, bytes []byte) bool {
		switch field {
		case 1:
			key = bytes
		case 2:
			entry.Account = string(bytes)
		case 3:
			entry.Name = string(bytes)
		// 0 unspecified, 1 SHA1, 2 SHA256, 3 SHA512, 4 MD5.
		case 4:
			switch value {
			case 0, 1:
				entry.Algorithm = SHA1
			case 2:
				entry.Algorithm = SHA256
			case 3:
				entry.Algorithm = SHA512
			case 4:
				bad(ReasonBadAlgorithm, "MD5")
			default:
				bad(ReasonBadAlgorithm, strconv.FormatUint(value, 10))
			}
		// 0 unspecified, 1 six, 2 eight.
		case 5:
			switch value {
			case 0, 1:
				entry.Digits = 6
			case 2:
				entry.Digits = 8
			default:
				bad(ReasonBadDigits, strconv.FormatUint(value, 10))
			}
		// 0 unspecified, 1 HOTP, 2 TOTP.
		case 6:
			switch value {
			case 0, 2:
				entry.Kind = KindTOTP
			case 1:
				entry.Kind = KindHOTP
			default:
				bad(ReasonBadKind, strconv.FormatUint(value, 10))
			}
		case 7:
			entry.Counter = value
		}
		return true
	})
	if !ok {
		return Entry{}, nil, false
	}
	entry.Name, entry.Account = strings.TrimSpace(entry.Name), strings.TrimSpace(entry.Account)
	if entry.Name == "" {
		entry.Name, entry.Account = entry.Account, ""
	}
	// The period is TOTP's alone and the counter HOTP's, as in a link.
	if entry.Kind == KindHOTP {
		entry.Period = 0
	} else {
		entry.Counter = 0
	}
	if len(key) == 0 {
		bad(ReasonNoSecret, "")
	}
	if problem != nil {
		problem.Name = entry.Name
		return Entry{}, problem, true
	}
	entry.Secret = encodeBase32(key)
	return entry, nil, true
}

// forEachField walks the fields of a protobuf message: a varint's value, or the bytes of a length-delimited field.
// It stops when visit says so or the message is broken, and tells whether it was whole.
func forEachField(data []byte, visit func(field int, value uint64, bytes []byte) bool) bool {
	for len(data) > 0 {
		tag, n := varint(data)
		if n == 0 {
			return false
		}
		data = data[n:]
		field := int(tag >> 3)
		var value uint64
		var bytes []byte
		switch tag & 7 {
		case 0:
			if value, n = varint(data); n == 0 {
				return false
			}
		case 1:
			n = 8
		case 2:
			length, m := varint(data)
			if m == 0 || length > uint64(len(data)-m) {
				return false
			}
			bytes = data[m : m+int(length)]
			if bytes == nil {
				bytes = []byte{}
			}
			n = m + int(length)
		case 5:
			n = 4
		default:
			return false
		}
		if n > len(data) {
			return false
		}
		data = data[n:]
		if !visit(field, value, bytes) {
			return false
		}
	}
	return true
}

// varint reads a protobuf varint and its length, 0 when it is cut short or too long.
func varint(data []byte) (uint64, int) {
	var value uint64
	for i, b := range data {
		if i == 10 {
			return 0, 0
		}
		value |= uint64(b&0x7f) << (7 * i)
		if b&0x80 == 0 {
			return value, i + 1
		}
	}
	return 0, 0
}

// jsonEntry is a code in JSON of Poros. Numbers may come as text, as other apps write them.
type jsonEntry struct {
	Name      string      `json:"name"`
	Account   string      `json:"account"`
	Secret    string      `json:"secret"`
	Kind      string      `json:"kind"`
	Algorithm string      `json:"algorithm"`
	Digits    json.Number `json:"digits"`
	Period    json.Number `json:"period"`
	Counter   json.Number `json:"counter"`
}

// parseJSON reads a list of codes, or an object with the list under "otp", as a backup of Poros holds it.
func parseJSON(text string) ([]Entry, []Problem) {
	var items []json.RawMessage
	if strings.HasPrefix(text, "{") {
		var object struct {
			OTP []json.RawMessage `json:"otp"`
		}
		if err := json.Unmarshal([]byte(text), &object); err != nil {
			return nil, []Problem{{Reason: ReasonBadJSON}}
		}
		items = object.OTP
	} else if err := json.Unmarshal([]byte(text), &items); err != nil {
		return nil, []Problem{{Reason: ReasonBadJSON}}
	}
	if len(items) == 0 {
		return nil, []Problem{{Reason: ReasonBadJSON}}
	}
	var entries []Entry
	var problems []Problem
	for _, raw := range items {
		var item jsonEntry
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err := decoder.Decode(&item); err != nil {
			problems = append(problems, Problem{Reason: ReasonBadJSON})
			continue
		}
		entry, problem := item.entry()
		if problem != nil {
			problems = append(problems, *problem)
			continue
		}
		entries = append(entries, entry)
	}
	return entries, problems
}

func (j jsonEntry) entry() (Entry, *Problem) {
	entry := New("")
	entry.Name, entry.Account = strings.TrimSpace(j.Name), strings.TrimSpace(j.Account)
	fail := func(reason, value string) (Entry, *Problem) {
		return Entry{}, &Problem{Name: entry.Name, Reason: reason, Value: value}
	}
	if strings.TrimSpace(j.Secret) == "" {
		return fail(ReasonNoSecret, "")
	}
	var ok bool
	if entry.Secret, ok = Normalize(j.Secret); !ok {
		return fail(ReasonBadSecret, "")
	}
	if j.Kind != "" {
		entry.Kind = strings.ToLower(j.Kind)
	}
	if entry.Kind != KindTOTP && entry.Kind != KindHOTP {
		return fail(ReasonBadKind, j.Kind)
	}
	if entry.Algorithm, ok = algorithmOf(j.Algorithm); !ok {
		return fail(ReasonBadAlgorithm, j.Algorithm)
	}
	number := func(n json.Number, reason string, low, high int64) (int64, *Problem) {
		value, err := strconv.ParseInt(strings.TrimSpace(n.String()), 10, 64)
		if err != nil || value < low || value > high {
			return 0, &Problem{Name: entry.Name, Reason: reason, Value: n.String()}
		}
		return value, nil
	}
	if j.Digits != "" {
		digits, problem := number(j.Digits, ReasonBadDigits, MinDigits, MaxDigits)
		if problem != nil {
			return Entry{}, problem
		}
		entry.Digits = int(digits)
	}
	if entry.Kind == KindHOTP {
		entry.Period = 0
		if j.Counter != "" {
			counter, err := strconv.ParseUint(strings.TrimSpace(j.Counter.String()), 10, 64)
			if err != nil {
				return fail(ReasonBadCounter, j.Counter.String())
			}
			entry.Counter = counter
		}
	} else if j.Period != "" {
		period, problem := number(j.Period, ReasonBadPeriod, MinPeriod, MaxPeriod)
		if problem != nil {
			return Entry{}, problem
		}
		entry.Period = int(period)
	}
	return entry, nil
}
