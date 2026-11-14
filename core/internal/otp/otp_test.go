package otp

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"
)

func base32Of(text string) string { return encodeBase32([]byte(text)) }

func TestHOTPOfRFC4226(t *testing.T) {
	entry := New(base32Of("12345678901234567890"))
	want := []string{"755224", "287082", "359152", "969429", "338314", "254676", "287922", "162583", "399871", "520489"}
	for counter, code := range want {
		if got := entry.Code(uint64(counter)); got != code {
			t.Errorf("counter %d: %s, want %s", counter, got, code)
		}
	}
}

func TestTOTPOfRFC6238(t *testing.T) {
	keys := map[string]string{
		SHA1:   "12345678901234567890",
		SHA256: "12345678901234567890123456789012",
		SHA512: "1234567890123456789012345678901234567890123456789012345678901234",
	}
	cases := []struct {
		at    int64
		codes map[string]string
	}{
		{59, map[string]string{SHA1: "94287082", SHA256: "46119246", SHA512: "90693936"}},
		{1111111109, map[string]string{SHA1: "07081804", SHA256: "68084774", SHA512: "25091201"}},
		{1111111111, map[string]string{SHA1: "14050471", SHA256: "67062674", SHA512: "99943326"}},
		{1234567890, map[string]string{SHA1: "89005924", SHA256: "91819424", SHA512: "93441116"}},
		{2000000000, map[string]string{SHA1: "69279037", SHA256: "90698825", SHA512: "38618901"}},
		{20000000000, map[string]string{SHA1: "65353130", SHA256: "77737706", SHA512: "47863826"}},
	}
	for _, c := range cases {
		for algorithm, code := range c.codes {
			entry := New(base32Of(keys[algorithm]))
			entry.Algorithm, entry.Digits = algorithm, 8
			got, until := entry.TOTP(time.Unix(c.at, 0))
			if got != code {
				t.Errorf("%s at %d: %s, want %s", algorithm, c.at, got, code)
			}
			if want := (c.at/30 + 1) * 30; until.Unix() != want {
				t.Errorf("%s at %d: until %d, want %d", algorithm, c.at, until.Unix(), want)
			}
		}
	}
}

func TestNormalizeTakesWhatAppsPrint(t *testing.T) {
	for _, secret := range []string{"JBSWY3DPEHPK3PXP", "jbsw y3dp ehpk 3pxp", "JBSW-Y3DP-EHPK-3PXP", "JBSWY3DPEHPK3PXP===="} {
		if got, ok := Normalize(secret); !ok || got != "JBSWY3DPEHPK3PXP" {
			t.Errorf("%q: %q %t", secret, got, ok)
		}
	}
	for _, secret := range []string{"", "JBSWY3DPEHPK3PX1", "секрет"} {
		if _, ok := Normalize(secret); ok {
			t.Errorf("%q is taken", secret)
		}
	}
}

func TestCheckNamesTheWrongValue(t *testing.T) {
	good := New("JBSWY3DPEHPK3PXP")
	if p := good.Check(); p != nil {
		t.Fatalf("a good entry: %v", p)
	}
	cases := map[string]func(*Entry){
		ReasonNoSecret:     func(e *Entry) { e.Secret = "" },
		ReasonBadSecret:    func(e *Entry) { e.Secret = "jbswy3dpehpk3pxp" },
		ReasonBadKind:      func(e *Entry) { e.Kind = "motp" },
		ReasonBadAlgorithm: func(e *Entry) { e.Algorithm = "MD5" },
		ReasonBadDigits:    func(e *Entry) { e.Digits = 11 },
		ReasonBadPeriod:    func(e *Entry) { e.Period = 0 },
	}
	for reason, change := range cases {
		entry := good
		change(&entry)
		if p := entry.Check(); p == nil || p.Reason != reason {
			t.Errorf("%s: %v", reason, p)
		}
	}
	hotp := good
	hotp.Kind, hotp.Period = KindHOTP, 0
	if p := hotp.Check(); p != nil {
		t.Errorf("HOTP needs no period: %v", p)
	}
}

func TestLinkThereAndBack(t *testing.T) {
	entry := Entry{Name: "Панель провайдера", Account: "user@example.com", Secret: "JBSWY3DPEHPK3PXP", Kind: KindTOTP, Algorithm: SHA256, Digits: 8, Period: 60}
	got, problem := ParseLink(entry.Link())
	if problem != nil || got != entry {
		t.Fatalf("%s: %+v %v", entry.Link(), got, problem)
	}
	hotp := Entry{Name: "Офис", Secret: "JBSWY3DPEHPK3PXP", Kind: KindHOTP, Algorithm: SHA1, Digits: 6, Counter: 12}
	if got, problem = ParseLink(hotp.Link()); problem != nil || got != hotp {
		t.Fatalf("%s: %+v %v", hotp.Link(), got, problem)
	}
}

func TestParseLinkLikeAuthenticators(t *testing.T) {
	entry, problem := ParseLink("otpauth://totp/ACME%20Co:john.doe@email.com?secret=HXDMVJECJJWSRB3HWIZR4IFUGFTMXBOZ&issuer=ACME%20Co&algorithm=SHA1&digits=6&period=30")
	if problem != nil || entry.Name != "ACME Co" || entry.Account != "john.doe@email.com" || entry.Secret != "HXDMVJECJJWSRB3HWIZR4IFUGFTMXBOZ" {
		t.Fatalf("%+v %v", entry, problem)
	}
	// The issuer parameter names the service; a label without one names the account alone.
	if entry, _ = ParseLink("otpauth://totp/Old:alice?secret=JBSWY3DPEHPK3PXP&issuer=New"); entry.Name != "New" || entry.Account != "alice" {
		t.Errorf("issuer over label: %+v", entry)
	}
	if entry, _ = ParseLink("otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP"); entry.Name != "alice" || entry.Account != "" {
		t.Errorf("label alone: %+v", entry)
	}
	for link, reason := range map[string]string{
		"otpauth://motp/x?secret=JBSWY3DPEHPK3PXP":               ReasonBadKind,
		"otpauth://totp/x":                                       ReasonNoSecret,
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PX1":               ReasonBadSecret,
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&algorithm=MD5": ReasonBadAlgorithm,
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&digits=3":      ReasonBadDigits,
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&period=0":      ReasonBadPeriod,
		"otpauth://hotp/x?secret=JBSWY3DPEHPK3PXP&counter=-1":    ReasonBadCounter,
	} {
		if _, problem := ParseLink(link); problem == nil || problem.Reason != reason {
			t.Errorf("%s: %v, want %s", link, problem, reason)
		}
	}
}

// migration writes a MigrationPayload as Google Authenticator exports it.
func migration(entries ...[]byte) string {
	var payload []byte
	for _, e := range entries {
		payload = appendBytes(payload, 1, e)
	}
	payload = appendVarint(payload, 2, 1)
	return "otpauth-migration://offline?data=" + url.QueryEscape(base64.StdEncoding.EncodeToString(payload))
}

func otpParameters(secret, name, issuer string, algorithm, digits, kind, counter uint64) []byte {
	var p []byte
	p = appendBytes(p, 1, []byte(secret))
	p = appendBytes(p, 2, []byte(name))
	if issuer != "" {
		p = appendBytes(p, 3, []byte(issuer))
	}
	p = appendVarint(p, 4, algorithm)
	p = appendVarint(p, 5, digits)
	p = appendVarint(p, 6, kind)
	if counter > 0 {
		p = appendVarint(p, 7, counter)
	}
	return p
}

func appendRaw(out []byte, v uint64) []byte {
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	return append(out, byte(v))
}

func appendVarint(out []byte, field int, v uint64) []byte {
	return appendRaw(appendRaw(out, uint64(field)<<3), v)
}

func appendBytes(out []byte, field int, b []byte) []byte {
	out = appendRaw(appendRaw(out, uint64(field)<<3|2), uint64(len(b)))
	return append(out, b...)
}

func TestParseMigrationFromGoogleAuthenticator(t *testing.T) {
	link := migration(
		otpParameters("12345678901234567890", "alice@example.com", "GitHub", 1, 1, 2, 0),
		otpParameters("abcdefghijklmnopqrst", "vpn", "", 2, 2, 1, 7),
		otpParameters("abcdefghijklmnopqrst", "old", "", 4, 1, 2, 0),
	)
	entries, problems := Parse(link)
	if len(entries) != 2 || len(problems) != 1 {
		t.Fatalf("entries %+v, problems %+v", entries, problems)
	}
	if e := entries[0]; e.Name != "GitHub" || e.Account != "alice@example.com" || e.Secret != base32Of("12345678901234567890") || e.Kind != KindTOTP || e.Digits != 6 || e.Period != DefaultPeriod {
		t.Errorf("first: %+v", e)
	}
	if e := entries[1]; e.Name != "vpn" || e.Kind != KindHOTP || e.Algorithm != SHA256 || e.Digits != 8 || e.Counter != 7 || e.Period != 0 {
		t.Errorf("second: %+v", e)
	}
	if p := problems[0]; p.Line != 1 || p.Name != "old" || p.Reason != ReasonBadAlgorithm || p.Value != "MD5" {
		t.Errorf("problem: %+v", p)
	}
	// Plus signs left unescaped become spaces in a query, and a broken payload is one problem.
	raw := strings.Replace(link, "%2B", "+", -1)
	if entries, _ = Parse(raw); len(entries) != 2 {
		t.Errorf("unescaped link: %+v", entries)
	}
	if _, problems = Parse("otpauth-migration://offline?data=CgQKAg"); len(problems) != 1 || problems[0].Reason != ReasonBadLink {
		t.Errorf("broken payload: %+v", problems)
	}
}

func TestParseTellsEachLine(t *testing.T) {
	text := strings.Join([]string{
		"otpauth://totp/Panel:user@example.com?secret=GEZDGNBVGY3TQOJQ&issuer=Panel",
		"",
		"KRSX G5BA MFZX G5BA",
		"JBSWY3DP",
		"otpauth://totp/Mail:admin@example.org?issuer=Mail",
		"просто текст тут",
	}, "\r\n")
	entries, problems := Parse(text)
	if len(entries) != 2 || entries[0].Name != "Panel" || entries[1].Secret != "KRSXG5BAMFZXG5BA" || entries[1].Name != "" {
		t.Fatalf("entries: %+v", entries)
	}
	want := []Problem{{Line: 4, Reason: ReasonShortSecret}, {Line: 5, Name: "Mail", Reason: ReasonNoSecret}, {Line: 6, Reason: ReasonUnknown}}
	if len(problems) != len(want) {
		t.Fatalf("problems: %+v", problems)
	}
	for i := range want {
		if problems[i] != want[i] {
			t.Errorf("problem %d: %+v, want %+v", i, problems[i], want[i])
		}
	}
}

func TestParseJSONOfPoros(t *testing.T) {
	text := `{"otp": [
		{"name": "GitHub", "account": "poros-dev", "secret": "jbsw y3dp ehpk 3pxp", "kind": "totp", "algorithm": "SHA256", "digits": 8, "period": "60"},
		{"name": "Офис", "secret": "JBSWY3DPEHPK3PXP", "kind": "hotp", "counter": 5},
		{"name": "Плохой", "secret": "JBSWY3DPEHPK3PXP", "digits": 12}
	]}`
	entries, problems := Parse(text)
	if len(entries) != 2 || len(problems) != 1 {
		t.Fatalf("entries %+v, problems %+v", entries, problems)
	}
	want := Entry{Name: "GitHub", Account: "poros-dev", Secret: "JBSWY3DPEHPK3PXP", Kind: KindTOTP, Algorithm: SHA256, Digits: 8, Period: 60}
	if entries[0] != want {
		t.Errorf("first: %+v", entries[0])
	}
	if e := entries[1]; e.Kind != KindHOTP || e.Counter != 5 || e.Period != 0 || e.Digits != 6 {
		t.Errorf("second: %+v", e)
	}
	if p := problems[0]; p.Name != "Плохой" || p.Reason != ReasonBadDigits || p.Value != "12" {
		t.Errorf("problem: %+v", p)
	}
	if _, problems = Parse("[1, 2"); len(problems) != 1 || problems[0].Reason != ReasonBadJSON {
		t.Errorf("broken JSON: %+v", problems)
	}
}
