package otp

import (
	"encoding/base64"
	"net/url"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// hangLimit bounds one input. The fuzzing engine has no time limit of its own.
const hangLimit = 10 * time.Second

// watchHang crashes the process when an input runs past hangLimit, which keeps the input as a crasher.
func watchHang() (stop func()) {
	timer := time.AfterFunc(hangLimit, func() {
		debug.SetTraceback("all")
		panic("the input ran longer than " + hangLimit.String())
	})
	return func() { timer.Stop() }
}

// checkParsed holds Parse to its word: every entry it gives counts codes, writes an otpauth:// link that reads back
// into the same entry, and every problem names a reason.
func checkParsed(t *testing.T, entries []Entry, problems []Problem) {
	t.Helper()
	for _, e := range entries {
		if problem := e.Check(); problem != nil {
			t.Fatalf("Parse gave %+v, which Check refuses: %v", e, problem)
		}
		if code := e.Code(e.Counter); len(code) != e.Digits {
			t.Fatalf("%+v counts %q", e, code)
		}
		link := e.Link()
		back, problem := ParseLink(link)
		if problem != nil {
			t.Fatalf("%+v is written as %q, which does not read back: %v", e, link, problem)
		}
		want := e
		// A lone label names the service, so an account without a name comes back as the name.
		if e.Name == "" {
			want.Name, want.Account = back.Name, back.Account
		}
		if back != want {
			t.Fatalf("%+v is written as %q, which reads as %+v", e, link, back)
		}
	}
	for _, p := range problems {
		if p.Reason == "" {
			t.Fatalf("a problem without a reason: %+v", p)
		}
	}
}

// Codes come from QR codes, files and the clipboard: whatever the text, Parse gives usable entries and problems.
func FuzzParse(f *testing.F) {
	for _, text := range []string{
		"otpauth://totp/ACME%20Co:john.doe@email.com?secret=HXDMVJECJJWSRB3HWIZR4IFUGFTMXBOZ&issuer=ACME%20Co&algorithm=SHA1&digits=6&period=30",
		"otpauth://totp/Old:alice?secret=JBSWY3DPEHPK3PXP&issuer=New",
		"otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP",
		"otpauth://hotp/%D0%9E%D1%84%D0%B8%D1%81?secret=JBSWY3DPEHPK3PXP&algorithm=SHA512&digits=8&counter=12",
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&algorithm=SHA-256&digits=10&period=3600",
		"otpauth://motp/x?secret=JBSWY3DPEHPK3PXP",
		"otpauth://totp/x",
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PX1",
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&algorithm=MD5",
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&digits=3",
		"otpauth://hotp/x?secret=JBSWY3DPEHPK3PXP&counter=-1",
		strings.Join([]string{
			"otpauth://totp/Panel:user@example.com?secret=GEZDGNBVGY3TQOJQ&issuer=Panel", "", "KRSX G5BA MFZX G5BA", "JBSWY3DP",
			"otpauth://totp/Mail:admin@example.org?issuer=Mail", "просто текст тут",
		}, "\r\n"),
		"jbsw-y3dp-ehpk-3pxp\nJBSWY3DPEHPK3PXP====",
		`{"otp": [
			{"name": "GitHub", "account": "poros-dev", "secret": "jbsw y3dp ehpk 3pxp", "kind": "totp", "algorithm": "SHA256", "digits": 8, "period": "60"},
			{"name": "Офис", "secret": "JBSWY3DPEHPK3PXP", "kind": "hotp", "counter": 5},
			{"name": "Плохой", "secret": "JBSWY3DPEHPK3PXP", "digits": 12}]}`,
		`[{"name": "A", "secret": "JBSWY3DPEHPK3PXP", "counter": "18446744073709551615", "kind": "HOTP"}, 1, null]`,
		"[1, 2",
		"otpauth-migration://offline?data=CgQKAg",
		migration(otpParameters("12345678901234567890", "alice@example.com", "GitHub", 1, 1, 2, 0),
			otpParameters("abcdefghijklmnopqrst", "vpn", "", 2, 2, 1, 7)),
	} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		defer watchHang()()
		entries, problems := Parse(text)
		checkParsed(t, entries, problems)
	})
}

// Google Authenticator's export is a protobuf inside the link: the fuzzer changes its bytes directly.
func FuzzParseMigration(f *testing.F) {
	for _, entries := range [][][]byte{
		{otpParameters("12345678901234567890", "alice@example.com", "GitHub", 1, 1, 2, 0)},
		{otpParameters("abcdefghijklmnopqrst", "vpn", "", 2, 2, 1, 7), otpParameters("abcdefghijklmnopqrst", "old", "", 4, 1, 2, 0)},
		{otpParameters("", "empty", "", 0, 0, 0, 0)},
		{otpParameters("k", " spaced : name ", "Issuer:With:Colons", 3, 9, 7, 1<<63)},
	} {
		link := migration(entries...)
		u, _ := url.Parse(link)
		payload, _ := base64.StdEncoding.DecodeString(u.Query().Get("data"))
		f.Add(payload)
	}
	f.Add([]byte{0x0a, 0x04, 0x0a, 0x02})
	f.Fuzz(func(t *testing.T, payload []byte) {
		defer watchHang()()
		link := "otpauth-migration://offline?data=" + url.QueryEscape(base64.StdEncoding.EncodeToString(payload))
		entries, problems := Parse(link)
		checkParsed(t, entries, problems)
	})
}
