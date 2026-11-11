// Package otp counts the one-time codes of sign-ins, TOTP and HOTP (RFC 6238 and 4226), and reads them from otpauth://
// links, Google Authenticator's otpauth-migration:// links, JSON and bare secrets. It carries over the OTP of the Qt
// version (qt-final:src/global/OTP.cpp).
package otp

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	KindTOTP = "totp"
	KindHOTP = "hotp"

	SHA1   = "SHA1"
	SHA256 = "SHA256"
	SHA512 = "SHA512"

	DefaultDigits = 6
	DefaultPeriod = 30
	MinDigits     = 4
	MaxDigits     = 10
	MinPeriod     = 1
	MaxPeriod     = 3600

	// minBareSecret is the bytes a bare secret needs: shorter base32 is more likely a word than a key.
	minBareSecret = 10
)

// Reasons of a Problem, which the interface translates.
const (
	ReasonNoSecret     = "no_secret"
	ReasonBadSecret    = "bad_secret"
	ReasonShortSecret  = "short_secret"
	ReasonBadKind      = "bad_kind"
	ReasonBadAlgorithm = "bad_algorithm"
	ReasonBadDigits    = "bad_digits"
	ReasonBadPeriod    = "bad_period"
	ReasonBadCounter   = "bad_counter"
	ReasonBadLink      = "bad_link"
	ReasonBadJSON      = "bad_json"
	// ReasonUnknown is a line that is no link, JSON or secret.
	ReasonUnknown = "unknown"
)

// Entry is one code: TOTP counts steps of Period seconds, HOTP its Counter, which grows with each code.
type Entry struct {
	// Name is the service, Account the user's name there; a link's label is "Name:Account".
	Name      string
	Account   string
	Secret    string
	Kind      string
	Algorithm string
	Digits    int
	Period    int
	Counter   uint64
}

// Problem is why an entry, or a line of a text, cannot be taken. Value is the wrong value, never a secret.
type Problem struct {
	Line   int
	Name   string
	Reason string
	Value  string
}

func (p Problem) Error() string {
	text := p.Reason
	if p.Value != "" {
		text += " " + strconv.Quote(p.Value)
	}
	if p.Name != "" {
		text = p.Name + ": " + text
	}
	return text
}

// New is an entry with the values most services use, for a secret alone.
func New(secret string) Entry {
	return Entry{Secret: secret, Kind: KindTOTP, Algorithm: SHA1, Digits: DefaultDigits, Period: DefaultPeriod}
}

// Check tells what makes the entry unusable, nil when its codes would be right. The secret must be the canonical one
// Normalize gives.
func (e Entry) Check() *Problem {
	fail := func(reason, value string) *Problem { return &Problem{Name: e.Name, Reason: reason, Value: value} }
	switch {
	case strings.TrimSpace(e.Secret) == "":
		return fail(ReasonNoSecret, "")
	case e.Kind != KindTOTP && e.Kind != KindHOTP:
		return fail(ReasonBadKind, e.Kind)
	case e.Algorithm != SHA1 && e.Algorithm != SHA256 && e.Algorithm != SHA512:
		return fail(ReasonBadAlgorithm, e.Algorithm)
	case e.Digits < MinDigits || e.Digits > MaxDigits:
		return fail(ReasonBadDigits, strconv.Itoa(e.Digits))
	case e.Kind == KindTOTP && (e.Period < MinPeriod || e.Period > MaxPeriod):
		return fail(ReasonBadPeriod, strconv.Itoa(e.Period))
	}
	if secret, ok := Normalize(e.Secret); !ok || secret != e.Secret {
		return fail(ReasonBadSecret, "")
	}
	return nil
}

// Code is the HOTP code of a counter; TOTP is the same over the number of the time step.
func (e Entry) Code(counter uint64) string {
	key, _ := decodeBase32(e.Secret)
	var mac hash.Hash
	switch e.Algorithm {
	case SHA256:
		mac = hmac.New(sha256.New, key)
	case SHA512:
		mac = hmac.New(sha512.New, key)
	default:
		mac = hmac.New(sha1.New, key)
	}
	var message [8]byte
	binary.BigEndian.PutUint64(message[:], counter)
	mac.Write(message[:])
	sum := mac.Sum(nil)
	// RFC 4226 dynamic truncation: the last nibble picks four bytes, without their top bit.
	offset := sum[len(sum)-1] & 0x0f
	value := uint64(binary.BigEndian.Uint32(sum[offset:]) & 0x7fffffff)
	modulus := uint64(1)
	for range e.Digits {
		modulus *= 10
	}
	return fmt.Sprintf("%0*d", e.Digits, value%modulus)
}

// TOTP is the code of the moment and when it gives way to the next one.
func (e Entry) TOTP(now time.Time) (string, time.Time) {
	period := int64(max(e.Period, MinPeriod))
	step := now.Unix() / period
	return e.Code(uint64(step)), time.Unix((step+1)*period, 0)
}

// Link writes the entry as an otpauth:// link, which authenticator apps read from a QR code.
func (e Entry) Link() string {
	label := e.Name
	if e.Account != "" {
		label = e.Name + ":" + e.Account
		if e.Name == "" {
			label = e.Account
		}
	}
	query := url.Values{}
	query.Set("secret", e.Secret)
	if e.Name != "" && e.Account != "" {
		query.Set("issuer", e.Name)
	}
	query.Set("algorithm", e.Algorithm)
	query.Set("digits", strconv.Itoa(e.Digits))
	if e.Kind == KindHOTP {
		query.Set("counter", strconv.FormatUint(e.Counter, 10))
	} else {
		query.Set("period", strconv.Itoa(e.Period))
	}
	u := url.URL{Scheme: "otpauth", Host: e.Kind, Path: "/" + label, RawQuery: query.Encode()}
	return u.String()
}

// Normalize gives the canonical form of a base32 secret, upper case without spaces, dashes or padding; false when
// it is not base32 or holds nothing.
func Normalize(secret string) (string, bool) {
	key, ok := decodeBase32(secret)
	if !ok || len(key) == 0 {
		return "", false
	}
	return encodeBase32(key), true
}

const base32Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

// decodeBase32 reads base32 of either case, skipping spaces, dashes, underscores and padding, and keeps the whole
// bytes of a length the standard decoder would refuse, as authenticator apps do.
func decodeBase32(text string) ([]byte, bool) {
	var out []byte
	var buffer uint32
	bits := 0
	for _, c := range text {
		var value uint32
		switch {
		case c == ' ' || c == '-' || c == '_' || c == '\t' || c == '\r' || c == '\n' || c == '=':
			continue
		case c >= 'A' && c <= 'Z':
			value = uint32(c - 'A')
		case c >= 'a' && c <= 'z':
			value = uint32(c - 'a')
		case c >= '2' && c <= '7':
			value = uint32(c-'2') + 26
		default:
			return nil, false
		}
		buffer = buffer<<5 | value
		bits += 5
		if bits >= 8 {
			bits -= 8
			out = append(out, byte(buffer>>bits))
		}
	}
	return out, true
}

func encodeBase32(data []byte) string {
	var out strings.Builder
	var buffer uint32
	bits := 0
	for _, b := range data {
		buffer = buffer<<8 | uint32(b)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out.WriteByte(base32Alphabet[buffer>>bits&0x1f])
		}
	}
	if bits > 0 {
		out.WriteByte(base32Alphabet[buffer<<(5-bits)&0x1f])
	}
	return out.String()
}
