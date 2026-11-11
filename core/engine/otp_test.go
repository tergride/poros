package engine

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"PorosCore/internal/jsonrpc"
	"PorosCore/internal/otp"
	"PorosCore/profile"
)

func TestOTPConstantsMatchTheContract(t *testing.T) {
	pairs := [][2]any{{OTPKindTOTP, otp.KindTOTP}, {OTPKindHOTP, otp.KindHOTP}, {OTPSHA1, otp.SHA1}, {OTPSHA256, otp.SHA256},
		{OTPSHA512, otp.SHA512}, {OTPMinDigits, otp.MinDigits}, {OTPMaxDigits, otp.MaxDigits}, {OTPMinPeriod, otp.MinPeriod},
		{OTPMaxPeriod, otp.MaxPeriod}}
	for _, p := range pairs {
		if p[0] != p[1] {
			t.Errorf("%v != %v", p[0], p[1])
		}
	}
}

func totp(name, secret string) OTPEntry {
	return OTPEntry{Name: name, Secret: secret, Kind: OTPKindTOTP, Algorithm: OTPSHA1, Digits: 6, Period: 30}
}

func TestOTPEntriesKeepTheirOrder(t *testing.T) {
	e, events := openTest(t, t.TempDir())
	first, err := e.SaveOTP(OTPEntry{Name: " GitHub ", Account: "poros-dev", Secret: "jbsw y3dp ehpk 3pxp", Kind: OTPKindTOTP, Algorithm: OTPSHA1, Digits: 6, Period: 30, Counter: 9})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.Name != "GitHub" || first.Secret != "JBSWY3DPEHPK3PXP" || first.Counter != 0 {
		t.Fatalf("a new entry is kept canonical: %+v", first)
	}
	second, err := e.SaveOTP(OTPEntry{Name: "Office", Secret: "KRSXG5BAMFZXG5BA", Kind: OTPKindHOTP, Algorithm: OTPSHA1, Digits: 6, Period: 30, Counter: 4})
	if err != nil {
		t.Fatal(err)
	}
	if second.Period != 0 || second.Counter != 4 {
		t.Fatalf("HOTP keeps its counter and no period: %+v", second)
	}

	codes, err := e.OTPCodes()
	if err != nil || len(codes) != 2 || codes[0].ID != first.ID || codes[1].ID != second.ID {
		t.Fatalf("list: %+v, %v", codes, err)
	}
	if c := codes[0]; len(c.Code) != 6 || c.Until <= time.Now().UnixMilli() || c.Until > time.Now().Add(30*time.Second).UnixMilli() {
		t.Errorf("a TOTP code with its end: %+v", c)
	}
	if c := codes[1]; c.Code != "" || c.Until != 0 {
		t.Errorf("a HOTP code comes from otp.code alone: %+v", c)
	}
	if last := (*events)[len(*events)-1]; last.method != EventOTPChanged {
		t.Errorf("a saved entry is announced: %+v", last)
	}

	if err = e.MoveOTP(OTPMove{ID: second.ID, Offset: -1}); err != nil {
		t.Fatal(err)
	}
	if err = e.MoveOTP(OTPMove{ID: second.ID, Offset: -1}); err != nil {
		t.Fatal(err)
	}
	if codes, _ = e.OTPCodes(); codes[0].ID != second.ID {
		t.Errorf("moved up, and stays first: %+v", codes)
	}
	first.Name = "GitHub, work"
	if _, err = e.SaveOTP(first); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.OTPEntry(first.ID); got.Name != "GitHub, work" || got.Secret != first.Secret {
		t.Errorf("edited in place: %+v", got)
	}
	if link, _ := e.OTPLink(first.ID); !strings.HasPrefix(link.Link, "otpauth://totp/") || !strings.Contains(link.Link, "secret=JBSWY3DPEHPK3PXP") {
		t.Errorf("link: %s", link.Link)
	}
	if err = e.RemoveOTP(second.ID); err != nil {
		t.Fatal(err)
	}
	if err = e.RemoveOTP(second.ID); reason(err) != ReasonNotFound {
		t.Errorf("removed twice: %v", err)
	}
	if codes, _ = e.OTPCodes(); len(codes) != 1 || codes[0].ID != first.ID {
		t.Errorf("after removal: %+v", codes)
	}
}

func TestOTPSaveNamesTheProblem(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	cases := map[string]OTPEntry{
		otp.ReasonBadSecret:    totp("a", "JBSWY3DPEHPK3PX1"),
		otp.ReasonNoSecret:     totp("a", ""),
		otp.ReasonBadDigits:    {Name: "a", Secret: "JBSWY3DPEHPK3PXP", Kind: OTPKindTOTP, Algorithm: OTPSHA1, Digits: 3, Period: 30},
		otp.ReasonBadAlgorithm: {Name: "a", Secret: "JBSWY3DPEHPK3PXP", Kind: OTPKindTOTP, Algorithm: "MD5", Digits: 6, Period: 30},
	}
	for want, entry := range cases {
		_, err := e.SaveOTP(entry)
		if reason(err) != ReasonInvalidOTP {
			t.Errorf("%s: %v", want, err)
		}
		if failure, ok := otpFailureOf(err); !ok || failure.Problem.Reason != want {
			t.Errorf("%s: %+v", want, failure)
		}
	}
	if codes, _ := e.OTPCodes(); len(codes) != 0 {
		t.Errorf("nothing is kept: %+v", codes)
	}
}

func TestOTPImportSkipsWhatIsThere(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	text := "otpauth://totp/GitHub:poros-dev?secret=JBSWY3DPEHPK3PXP&issuer=GitHub\nKRSX G5BA MFZX G5BA\nnot a code"
	parsed, _ := e.ParseOTP(OTPText{Text: text})
	if len(parsed.Entries) != 2 || len(parsed.Problems) != 1 || parsed.Problems[0].Line != 3 {
		t.Fatalf("parsed: %+v", parsed)
	}
	if codes, _ := e.OTPCodes(); len(codes) != 0 {
		t.Fatalf("parsing adds nothing: %+v", codes)
	}
	imported, err := e.ImportOTP(OTPText{Text: text})
	if err != nil || imported.Added != 2 || imported.Duplicates != 0 || len(imported.Problems) != 1 {
		t.Fatalf("imported: %+v, %v", imported, err)
	}
	if imported, _ = e.ImportOTP(OTPText{Text: text}); imported.Added != 0 || imported.Duplicates != 2 {
		t.Errorf("again: %+v", imported)
	}
	if codes, _ := e.OTPCodes(); len(codes) != 2 || codes[0].Name != "GitHub" || codes[0].Account != "poros-dev" {
		t.Errorf("list: %+v", codes)
	}
}

func TestOTPCodeSpendsHOTP(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	entry, err := e.SaveOTP(OTPEntry{Name: "Office", Secret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", Kind: OTPKindHOTP, Algorithm: OTPSHA1, Digits: 6})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"755224", "287082"} {
		if code, err := e.OTPCodeNow(entry.ID); err != nil || code.Code != want {
			t.Fatalf("code: %+v, %v, want %s", code, err, want)
		}
	}
	if got, _ := e.OTPEntry(entry.ID); got.Counter != 2 {
		t.Errorf("the counter is kept: %d", got.Counter)
	}
	totpEntry, _ := e.SaveOTP(totp("GitHub", "JBSWY3DPEHPK3PXP"))
	listed, _ := e.OTPCodes()
	if code, _ := e.OTPCodeNow(totpEntry.ID); code.Code != listed[1].Code {
		t.Errorf("TOTP gives the code of the moment: %+v, %+v", code, listed[1])
	}
}

func otpProfile(kind, password, otpID string) profile.Profile {
	p := profile.Profile{Type: kind, Name: "Office", Server: "vpn.example.org", ServerPort: 443}
	if kind == profile.TypeOpenVPN {
		p.OpenVPN = &profile.OpenVPN{Username: "u", Password: password, OTP: otpID}
	} else {
		p.OpenConnect = &profile.OpenConnect{Username: "{otp}", Password: password, OTP: otpID}
	}
	return p
}

func TestOTPFillsTheSignIn(t *testing.T) {
	hotp := OTPEntry{ID: "h", Secret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", Kind: OTPKindHOTP, Algorithm: OTPSHA1, Digits: 6}
	codes := &otpCodes{entries: []OTPEntry{hotp, totp("t", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")}, now: time.Unix(59, 0)}
	codes.entries[1].ID, codes.entries[1].Digits = "t", 8

	original := otpProfile(profile.TypeOpenVPN, "pw{otp}", "h")
	got, err := codes.fill(original)
	if err != nil || got.OpenVPN.Password != "pw755224" || got.OpenVPN.Username != "u" {
		t.Fatalf("HOTP: %+v, %v", got.OpenVPN, err)
	}
	if original.OpenVPN.Password != "pw{otp}" || !codes.spent || codes.entries[0].Counter != 1 {
		t.Errorf("the stored profile stays, the counter moves: %+v, %+v", original.OpenVPN, codes.entries[0])
	}
	if got, err = codes.fill(otpProfile(profile.TypeOpenConnect, "{otp}", "t")); err != nil || got.OpenConnect.Username != "94287082" || got.OpenConnect.Password != "94287082" {
		t.Errorf("TOTP in both fields: %+v, %v", got.OpenConnect, err)
	}
	plain := otpProfile(profile.TypeOpenVPN, "pw", "")
	if got, err = codes.fill(plain); err != nil || got.OpenVPN.Password != "pw" {
		t.Errorf("without {otp} nothing is asked: %+v, %v", got.OpenVPN, err)
	}
	for _, id := range []string{"", "gone"} {
		if _, err = codes.fill(otpProfile(profile.TypeOpenVPN, "{otp}", id)); !isConnReason(err, ReasonOTPMissing) {
			t.Errorf("code %q: %v", id, err)
		}
	}
	test := &otpCodes{entries: []OTPEntry{hotp}, now: time.Unix(59, 0), test: true}
	if _, err = test.fill(original); err == nil || test.spent {
		t.Errorf("a test does not spend HOTP: %v", err)
	}
}

func otpFailureOf(err error) (OTPFailure, bool) {
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		return OTPFailure{}, false
	}
	failure, ok := rpcErr.Data.(OTPFailure)
	return failure, ok
}

func isConnReason(err error, reason string) bool {
	failed, ok := err.(connError)
	return ok && failed.reason == reason
}

// The code of a HOTP entry signs in to a loopback OpenVPN server, and the next start takes the next code.
func TestConnectionSignsInWithOTP(t *testing.T) {
	e, states := openConnectionTest(t)
	entry, err := e.SaveOTP(OTPEntry{Name: "Office", Secret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", Kind: OTPKindHOTP, Algorithm: OTPSHA1, Digits: 6})
	if err != nil {
		t.Fatal(err)
	}
	page := insidePage(t)
	certificate, key := testCertificate(t)
	port := freePort(t)
	runServer(t, fmt.Sprintf(`{"log":{"level":"warn"},"endpoints":[{"type":"openvpn-server","tag":"ovpn-in","listen":"127.0.0.1",`+
		`"listen_port":%d,"network":"tcp","address":["10.8.0.1/24"],"users":[{"username":"u","password":"pw755224"}],`+
		`"tls":{"certificate":%s,"key":%s,"verify_client_certificate":"none"}}],"outbounds":[{"type":"direct","tag":"direct"}],%s}`,
		port, jsonText(t, certificate), jsonText(t, key), insideRoute))
	useServer(t, e, porosLink(t, fmt.Sprintf(`{"type":"openvpn","name":"local","server":"127.0.0.1","server_port":%d,`+
		`"openvpn":{"network":"tcp","username":"u","password":"pw{otp}","otp":%s,"ca":%s}}`, port, jsonText(t, entry.ID), jsonText(t, certificate))))
	connectAndFetch(t, e, states, page)
	if got, _ := e.OTPEntry(entry.ID); got.Counter != 1 {
		t.Errorf("the start spent one code: %d", got.Counter)
	}
	e.StopConnection()
	waitState(t, states)
	// The server takes only the first code: the next start spends the second and is refused.
	e.StartConnection()
	if state := waitState(t, states); state.State != ConnectionIdle || state.Error != ReasonStartFailed {
		t.Fatalf("a spent code: %+v", state)
	}
	if err = e.RemoveOTP(entry.ID); err != nil {
		t.Fatal(err)
	}
	e.StartConnection()
	if state := waitState(t, states); state.State != ConnectionIdle || state.Error != ReasonOTPMissing {
		t.Fatalf("a removed code: %+v", state)
	}
}

func TestTestsTakeNoHOTP(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	hotp, _ := e.SaveOTP(OTPEntry{Name: "Office", Secret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", Kind: OTPKindHOTP, Algorithm: OTPSHA1, Digits: 6})
	code, _ := e.SaveOTP(totp("Panel", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"))
	group, err := e.AddGroup(AddGroup{Name: "Mine"})
	if err != nil {
		t.Fatal(err)
	}
	certificate, _ := testCertificate(t)
	var ids []string
	for _, id := range []string{hotp.ID, code.ID} {
		p := otpProfile(profile.TypeOpenVPN, "pw{otp}", id)
		p.OpenVPN.CA = certificate
		saved, err := e.SaveServer(SaveServer{GroupID: group.ID, Profile: p})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, saved.ID)
	}
	servers, err := e.testServersOf(ServersQuery{IDs: ids})
	if err != nil || len(servers) != 2 {
		t.Fatalf("%+v, %v", servers, err)
	}
	if servers[0].Server.Type != "" {
		t.Errorf("a HOTP server is not tested: %+v", servers[0].Server)
	}
	if p := servers[1].Server.OpenVPN; p == nil || len(p.Password) != 8 || !strings.HasPrefix(p.Password, "pw") {
		t.Errorf("a TOTP server is tested with its code: %+v", p)
	}
	if got, _ := e.OTPEntry(hotp.ID); got.Counter != 0 {
		t.Errorf("a test spends nothing: %d", got.Counter)
	}
}
