package engine

import (
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sagernet/bbolt"

	"PorosCore/config"
	"PorosCore/internal/stb/stbkey"
	"PorosCore/internal/stb/stbtest"
	"PorosCore/profile"
)

// testEndpoint is an OpenVPN endpoint that signs in with a name, a password and a client pair, and sends example.org
// through itself.
func testEndpoint(t *testing.T, name string) EndpointEntry {
	t.Helper()
	certificate, key := testCertificate(t)
	return EndpointEntry{
		Profile: profile.Profile{Type: profile.TypeOpenVPN, Name: name, Server: "vpn.example.com", ServerPort: 1194, OpenVPN: &profile.OpenVPN{
			Network: "udp", Username: "ivanov", Password: "pw", CA: certificate, Cert: certificate, Key: key,
		}},
		Rules: []PresetRule{rule(CondSuffix, "example.org", ActionEndpoint)},
	}
}

func endpointNames(t *testing.T, e *Engine) []string {
	t.Helper()
	list, err := e.Endpoints()
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, endpoint := range list {
		names = append(names, endpoint.Name)
	}
	return names
}

// Endpoints keep the user's order, names that do not repeat and rules that go through them.
func TestEndpointsKeepTheirOrder(t *testing.T) {
	e, events := openTest(t, tempDir(t))
	office, err := e.SaveEndpoint(testEndpoint(t, "Office"))
	if err != nil {
		t.Fatal(err)
	}
	lab, err := e.SaveEndpoint(testEndpoint(t, "Lab"))
	if err != nil {
		t.Fatal(err)
	}
	if names := endpointNames(t, e); !slices.Equal(names, []string{"Office", "Lab"}) || office.Type != profile.TypeOpenVPN || office.Rules != 1 || office.On {
		t.Fatalf("a new endpoint comes last: %v, %+v", names, office)
	}
	if err = e.MoveEndpoint(EndpointMove{ID: lab.ID, Offset: -1}); err != nil || !slices.Equal(endpointNames(t, e), []string{"Lab", "Office"}) {
		t.Fatalf("an endpoint moves up: %v, %v", endpointNames(t, e), err)
	}
	if err = e.MoveEndpoint(EndpointMove{ID: lab.ID, Offset: -1}); err != nil || !slices.Equal(endpointNames(t, e), []string{"Lab", "Office"}) {
		t.Fatalf("past the top it stays: %v, %v", endpointNames(t, e), err)
	}

	entry, _ := e.EndpointEntry(office.ID)
	entry.On, entry.Profile.Name = true, "  Office  "
	entry.Rules = append(entry.Rules, rule(CondIP, "10.20.0.0/16", ActionEndpoint))
	if saved, err := e.SaveEndpoint(entry); err != nil || saved.Name != "Office" || !saved.On || saved.Rules != 2 {
		t.Fatalf("a known endpoint changes in its place: %+v, %v", saved, err)
	}
	if !slices.ContainsFunc(*events, func(ev event) bool { return ev.method == EventEndpointsChanged }) {
		t.Error("no event after a change")
	}

	taken := testEndpoint(t, "office")
	if _, err = e.SaveEndpoint(taken); reason(err) != ReasonEndpointNameTaken {
		t.Errorf("a name of another endpoint, whatever its case: %v", err)
	}
	missing := testEndpoint(t, "Gone")
	missing.ID = "gone"
	if _, err = e.SaveEndpoint(missing); reason(err) != ReasonNotFound {
		t.Errorf("an endpoint that is not there: %v", err)
	}
	if err = e.RemoveEndpoint(lab.ID); err != nil || !slices.Equal(endpointNames(t, e), []string{"Office"}) {
		t.Fatalf("an endpoint goes: %v, %v", endpointNames(t, e), err)
	}
	if err = e.RemoveEndpoint(lab.ID); reason(err) != ReasonNotFound {
		t.Errorf("an endpoint that went: %v", err)
	}
}

func TestEndpointRefusals(t *testing.T) {
	e, _ := openTest(t, tempDir(t))
	for name, change := range map[string]func(*EndpointEntry){
		"no VPN": func(entry *EndpointEntry) {
			entry.Profile = profile.Profile{Type: profile.TypeTrojan, Name: "Trojan", Server: "de.example.com", ServerPort: 443, Trojan: &profile.Trojan{Password: "pw"}}
		},
		"no name":            func(entry *EndpointEntry) { entry.Profile.Name = " " },
		"a broken profile":   func(entry *EndpointEntry) { entry.Profile.OpenVPN.CA = "" },
		"another action":     func(entry *EndpointEntry) { entry.Rules[0].Action = ActionDirect },
		"a rule without one": func(entry *EndpointEntry) { entry.Rules[0].Conditions = nil },
	} {
		entry := testEndpoint(t, "Office")
		change(&entry)
		if _, err := e.SaveEndpoint(entry); err == nil {
			t.Errorf("%s is saved", name)
		}
	}
	if _, err := CheckPresetRules(PresetRules{Advanced: true, Rules: []PresetRule{rule(CondSuffix, "example.org", ActionEndpoint)}}); err == nil {
		t.Error("a preset has no rule to an endpoint")
	}
	checked, err := CheckPresetRules(PresetRules{Endpoint: true, Rules: []PresetRule{rule(CondPort, "443", ActionEndpoint), rule(CondSuffix, "Example.ORG", ActionEndpoint)}})
	if err != nil || !slices.Equal(values(checked.Rules), []string{"port:443", "suffix:example.org"}) {
		t.Errorf("the rules of an endpoint take the kinds of an advanced preset: %+v, %v", checked, err)
	}
}

// A file without the data for signing in names what it left out; the import asks for exactly that, and the endpoint
// stays off until it has it. The code of OTP is this computer's and never goes into a file.
func TestEndpointFiles(t *testing.T) {
	e, _ := openTest(t, tempDir(t))
	code, _ := e.SaveOTP(totp("Office", "JBSWY3DPEHPK3PXP"))
	entry := testEndpoint(t, "Office")
	entry.On = true
	entry.Profile.OpenVPN.Password, entry.Profile.OpenVPN.OTP = "pw{otp}", code.ID
	saved, err := e.SaveEndpoint(entry)
	if err != nil {
		t.Fatal(err)
	}

	full, err := e.ExportEndpoint(EndpointExport{ID: saved.ID, SignIn: true})
	if err != nil || full.Name != "Office.json" || strings.Contains(full.Text, code.ID) {
		t.Fatalf("a file with the data for signing in, without the code: %+v, %v", full, err)
	}
	var file endpointFile
	if err = json.Unmarshal([]byte(full.Text), &file); err != nil || file.Kind != "poros-endpoint" || file.Profile.OpenVPN.Password != "pw{otp}" ||
		file.Profile.OpenVPN.Key == "" || !slices.Equal(file.Need, []string{SignInOTP}) || len(file.Rules) != 1 {
		t.Fatalf("the file: %+v, %v", file, err)
	}

	safe, err := e.ExportEndpoint(EndpointExport{ID: saved.ID})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(safe.Text, "ivanov") || strings.Contains(safe.Text, "PRIVATE KEY") || !strings.Contains(safe.Text, "CERTIFICATE") {
		t.Fatalf("a file without the data for signing in keeps the CA alone: %s", safe.Text)
	}
	imported, err := e.ImportEndpoint(EndpointText{Text: safe.Text})
	want := []string{SignInUsername, SignInPassword, SignInCert, SignInKey, SignInOTP}
	if err != nil || imported.Name != "Office 2" || imported.On || !slices.Equal(imported.Need, want) || imported.Rules != 1 {
		t.Fatalf("an import is off, needs what the file left out and takes a free name: %+v, %v", imported, err)
	}
	filled, _ := e.EndpointEntry(imported.ID)
	filled.On = true
	if _, err = e.SaveEndpoint(filled); reason(err) != ReasonSignInMissing {
		t.Errorf("on without the data for signing in: %v", err)
	}
	certificate, key := testCertificate(t)
	o := filled.Profile.OpenVPN
	o.Username, o.Password, o.Cert, o.Key = "petrov", "secret{otp}", certificate, key
	if saved, err := e.SaveEndpoint(filled); err == nil || reason(err) != ReasonSignInMissing {
		t.Errorf("the code of OTP is still missing: %+v, %v", saved, err)
	}
	o.OTP = code.ID
	if saved, err := e.SaveEndpoint(filled); err != nil || !saved.On || len(saved.Need) != 0 {
		t.Errorf("filled in, it turns on: %+v, %v", saved, err)
	}
	// Who signs in without {otp} needs no code of OTP, whatever the file asked for.
	plain, err := e.ImportEndpoint(EndpointText{Text: safe.Text})
	if err != nil {
		t.Fatal(err)
	}
	filled, _ = e.EndpointEntry(plain.ID)
	filled.On = true
	o = filled.Profile.OpenVPN
	o.Username, o.Password, o.Cert, o.Key = "sidorov", "secret", certificate, key
	if saved, err := e.SaveEndpoint(filled); err != nil || !saved.On || len(saved.Need) != 0 {
		t.Errorf("a password without {otp} turns on without a code: %+v, %v", saved, err)
	}

	if _, err = e.ImportEndpoint(EndpointText{Text: `{"kind":"poros-endpoint","v":1,"profile":{},"extra":true}`}); reason(err) != ReasonInvalidLink {
		t.Errorf("an unknown key: %v", err)
	}
	if _, err = e.ImportEndpoint(EndpointText{Text: "trojan://pw@de.example.com:443#Germany"}); reason(err) != ReasonInvalidLink {
		t.Errorf("a link to a server: %v", err)
	}
}

// The key of tls-crypt-v2 is the client's own: a file without the data for signing in leaves it out, and the
// endpoint reads without it until it is back.
func TestEndpointClientWrapKey(t *testing.T) {
	e, _ := openTest(t, tempDir(t))
	entry := testEndpoint(t, "Office")
	entry.Profile.OpenVPN.ControlWrap = profile.OpenVPNControlWrap{Type: "tls_crypt_v2", Key: "-----BEGIN OpenVPN tls-crypt-v2 client key-----\nAAAA\n-----END OpenVPN tls-crypt-v2 client key-----"}
	saved, err := e.SaveEndpoint(entry)
	if err != nil {
		t.Fatal(err)
	}
	safe, _ := e.ExportEndpoint(EndpointExport{ID: saved.ID})
	imported, err := e.ImportEndpoint(EndpointText{Text: safe.Text})
	if err != nil || !slices.Contains(imported.Need, SignInWrapKey) {
		t.Fatalf("the client's key is left out and needed: %+v, %v", imported, err)
	}
	shared := testEndpoint(t, "Shared")
	shared.Profile.OpenVPN.ControlWrap = profile.OpenVPNControlWrap{Type: "tls_crypt", Key: "-----BEGIN OpenVPN Static key V1-----\nAAAA\n-----END OpenVPN Static key V1-----"}
	saved, _ = e.SaveEndpoint(shared)
	if safe, _ = e.ExportEndpoint(EndpointExport{ID: saved.ID}); !strings.Contains(safe.Text, "Static key") {
		t.Errorf("the key of tls-crypt is the server's and stays: %s", safe.Text)
	}
}

func TestEndpointFromOVPN(t *testing.T) {
	e, _ := openTest(t, tempDir(t))
	certificate, _ := testCertificate(t)
	text := "client\ndev tun\nproto tcp\nremote vpn.example.com 443\nauth-user-pass\nsetenv FRIENDLY_NAME Office\n<ca>\n" + certificate + "</ca>\n"
	imported, err := e.ImportEndpoint(EndpointText{Text: text})
	if err != nil || imported.Name != "Office" || imported.On || imported.Rules != 0 || !slices.Equal(imported.Need, []string{SignInUsername, SignInPassword}) {
		t.Fatalf("an .ovpn that asks for a name and a password: %+v, %v", imported, err)
	}
	unnamed := strings.ReplaceAll(strings.ReplaceAll(text, "setenv FRIENDLY_NAME Office\n", ""), "auth-user-pass\n", "")
	if imported, err = e.ImportEndpoint(EndpointText{Text: unnamed}); err != nil || imported.Name != "vpn.example.com" || len(imported.Need) != 0 {
		t.Errorf("an .ovpn without a name and a sign-in: %+v, %v", imported, err)
	}
}

// testOVPNBY is a config of OpenVPN|BY as its client writes one, with the CA and the client's pair in files beside it.
const testOVPNBY = "\xef\xbb\xbfclient\r\ndev tun\r\nproto tcp\r\nremote by.example.com 443\r\nsetenv FRIENDLY_NAME Office\r\nca CA_crt.pem\r\n" +
	"cert client_crt.pem\r\nkey client_pr.pem\r\ntls-cipher DHE-BIGN-WITH-BELT-DWP-HBELT\r\ncipher belt-cfb256\r\n" +
	"data-ciphers belt-cfb256\r\nauth bash512\r\ntls-version-max 1.2\r\ntls-version-min 1.2\r\n"

// An .ovpn of OpenVPN|BY with its keys in files needs them, and the password of the key while the key has one or is
// not there to tell; a file without the data for signing in keeps the CA, the server's.
func TestEndpointFromOVPNBY(t *testing.T) {
	e, _ := openTest(t, tempDir(t))
	imported, err := e.ImportEndpoint(EndpointText{Text: testOVPNBY})
	want := []string{SignInCA, SignInCert, SignInKey, SignInKeyPassword}
	if err != nil || imported.Type != profile.TypeOpenVPNBY || imported.Name != "Office" || imported.On || !slices.Equal(imported.Need, want) {
		t.Fatalf("the files are needed: %+v, %v", imported, err)
	}
	certificate, _ := testCertificate(t)
	locked := "-----BEGIN ENCRYPTED PRIVATE KEY-----\nAAAA\n-----END ENCRYPTED PRIVATE KEY-----"
	entry, _ := e.EndpointEntry(imported.ID)
	o := entry.Profile.OpenVPN
	o.CA, o.Cert, o.Key = certificate, certificate, locked
	if saved, err := e.SaveEndpoint(entry); err != nil || !slices.Equal(saved.Need, []string{SignInKeyPassword}) {
		t.Fatalf("a key under a password needs it: %+v, %v", saved, err)
	}
	entry.On, o.KeyPassword = true, "secret"
	if saved, err := e.SaveEndpoint(entry); err != nil || !saved.On || len(saved.Need) != 0 {
		t.Fatalf("filled in, it turns on: %+v, %v", saved, err)
	}

	safe, err := e.ExportEndpoint(EndpointExport{ID: imported.ID})
	if err != nil || !strings.Contains(safe.Text, "BEGIN CERTIFICATE") || strings.Contains(safe.Text, "secret") || strings.Contains(safe.Text, "PRIVATE KEY") {
		t.Fatalf("a file without the data for signing in keeps the CA alone: %s, %v", safe.Text, err)
	}
	again, err := e.ImportEndpoint(EndpointText{Text: safe.Text})
	if err != nil || !slices.Equal(again.Need, []string{SignInCert, SignInKey, SignInKeyPassword}) {
		t.Fatalf("it comes back needing the client's own: %+v, %v", again, err)
	}
	entry, _ = e.EndpointEntry(again.ID)
	o = entry.Profile.OpenVPN
	o.Cert, o.Key = certificate, "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----"
	if saved, err := e.SaveEndpoint(entry); err != nil || len(saved.Need) != 0 {
		t.Errorf("an open key needs no password: %+v, %v", saved, err)
	}
}

// A key of OpenVPN|BY that its password does not open fails «Start» with a reason of its own and the server's name.
func TestStartRefusesAShutKey(t *testing.T) {
	e, states := openConnectionTest(t)
	client, err := stbtest.Issue(stbtest.Options{Name: "client", KeyUsage: x509.KeyUsageDigitalSignature}, nil)
	if err != nil {
		t.Fatal(err)
	}
	locked, err := stbkey.Encrypt(client.Key, []byte("secret"), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate := string(stbtest.PEM(client.Cert))
	useServer(t, e, porosLink(t, fmt.Sprintf(`{"type":"openvpn-by","name":"Office","server":"127.0.0.1","server_port":%d,"openvpn":{`+
		`"network":"tcp","ca":%s,"cert":%s,"key":%s,"key_password":"wrong","tls_cipher":["DHE-BIGN-WITH-BELT-DWP-HBELT"],`+
		`"data_ciphers":["belt-cbc256"],"auth":"bash512"}}`, freePort(t), jsonText(t, certificate), jsonText(t, certificate), jsonText(t, string(locked)))))
	e.StartConnection()
	if state := waitState(t, states); state.State != ConnectionIdle || state.Error != ReasonKeyPassword || !strings.HasPrefix(state.ErrorText, "Office: ") {
		t.Fatalf("a wrong password of the key: %+v", state)
	}
}

// An .ovpn that keeps its keys in files adds nothing yet: it goes to the editor, where they are chosen.
func TestAddServersOVPNDraft(t *testing.T) {
	e, _ := openTest(t, tempDir(t))
	added, err := e.AddServers(AddServers{NewGroup: "Mine", Text: testOVPNBY})
	if err != nil || added.Draft == nil || len(added.IDs) != 0 || added.Draft.Type != profile.TypeOpenVPNBY || added.Draft.Name != "Office" {
		t.Fatalf("a draft: %+v, %v", added, err)
	}
	if groups, _ := e.Groups(); slices.ContainsFunc(groups, func(g Group) bool { return g.Name == "Mine" }) {
		t.Error("a draft makes no group")
	}
	if _, err = e.AddServers(AddServers{NewGroup: "Mine", Text: "client\ndev tun\nremote v.example.com\n"}); reason(err) != ReasonNoServers {
		t.Errorf("an .ovpn without a CA and without files is no draft: %v", err)
	}
}

// The endpoints that are on come right after the DNS queries, in their order, before the preset's own rules; the
// connection reaches each by its place.
func TestConnectionPresetHasTheEndpointsOn(t *testing.T) {
	e, _ := openTest(t, tempDir(t))
	preset, err := e.SavePreset(PresetCopy{BasedOn: "all", Name: "Home", Rules: []PresetRule{rule(CondSuffix, "example.net", ActionDirect)}})
	if err != nil {
		t.Fatal(err)
	}
	add := func(name string, on bool, need []string) string {
		entry := testEndpoint(t, name)
		entry.On = on
		saved, err := e.SaveEndpoint(entry)
		if err != nil {
			t.Fatal(err)
		}
		if need != nil {
			// What a file left out keeps an endpoint out of the connection even when something set it on.
			_ = e.db.Update(func(tx *bbolt.Tx) error {
				records, _ := loadEndpoints(tx)
				records[len(records)-1].Need = need
				return putJSON(tx, bucketEndpoints, keyEndpoints, records)
			})
		}
		return saved.ID
	}
	office := add("Office", true, nil)
	add("Off", false, nil)
	add("Needs", true, []string{SignInPassword})
	lab := add("Lab", true, nil)

	built, endpoints, found, err := e.connectionPreset(preset.ID)
	if err != nil || !found || len(endpoints) != 2 || !slices.Equal(built.Endpoints, []string{office, lab}) {
		t.Fatalf("the endpoints that are on: %+v, %v", built.Endpoints, err)
	}
	var order []string
	for _, r := range built.Rules {
		order = append(order, r.Action+" "+r.Outbound+" "+r.Name)
	}
	want := []string{"hijack-dns  Route DNS", "route " + config.EndpointTag(0) + " example.org", "route " + config.EndpointTag(1) + " example.org",
		"route direct example.net"}
	if !reflect.DeepEqual(order[:len(want)], want) {
		t.Fatalf("the order of the rules: %q", order)
	}
	if plain, _, _, _ := e.connectionPreset("all"); !slices.Equal(plain.Endpoints, []string{office, lab}) || plain.Rules[1].Outbound != config.EndpointTag(0) {
		t.Errorf("a built-in preset runs with the endpoints too: %+v", plain)
	}
	if base, _ := e.builtin("all"); len(base.Preset.Endpoints) != 0 || len(base.Preset.Rules) != 2 {
		t.Errorf("the built-in preset itself stays as it is: %+v", base.Preset)
	}
}

func TestEndpointLists(t *testing.T) {
	rules := []PresetRule{rule(CondSuffix, "example.org", ActionEndpoint), rule(CondIP, "10.20.0.0/16", ActionEndpoint)}
	lists, err := WritePresetLists(PresetRules{Endpoint: true, Rules: rules})
	if err != nil || len(lists.Lists) != 1 || lists.Lists[ActionEndpoint] != "suffix:example.org\nip:10.20.0.0/16" {
		t.Fatalf("an endpoint has one list: %+v, %v", lists, err)
	}
	read, err := ReadPresetLists(PresetListsInput{Endpoint: true, Rules: rules, Lists: map[string]string{ActionEndpoint: "ip:10.20.0.0/16\nport:443"}})
	if err != nil || !slices.Equal(values(read.Rules), []string{"ip:10.20.0.0/16", "port:443"}) || read.Rules[1].Action != ActionEndpoint {
		t.Fatalf("the list of an endpoint: %+v, %v", read, err)
	}
	if _, err = ReadPresetLists(PresetListsInput{Endpoint: true, Lists: map[string]string{ActionProxy: "suffix:example.org"}}); reason(err) != ReasonInvalidValue {
		t.Errorf("an endpoint has no list of another action: %v", err)
	}
}

func TestEndpointsInTheBackup(t *testing.T) {
	source, _ := openTest(t, tempDir(t))
	entry := testEndpoint(t, "Office")
	entry.On = true
	saved, err := source.SaveEndpoint(entry)
	if err != nil {
		t.Fatal(err)
	}
	file, err := source.CreateBackup(BackupParts{Parts: []string{BackupRouting}})
	if err != nil {
		t.Fatal(err)
	}
	target, _ := openTest(t, tempDir(t))
	if _, err = target.SaveEndpoint(testEndpoint(t, "Mine")); err != nil {
		t.Fatal(err)
	}
	info, err := target.RestoreBackup(BackupRestore{Text: file.Text})
	if err != nil || info.Endpoints != 1 {
		t.Fatalf("restore: %+v, %v", info, err)
	}
	if list, _ := target.Endpoints(); len(list) != 1 || list[0].ID != saved.ID || !list[0].On || list[0].Name != "Office" {
		t.Errorf("the endpoints of the backup take the place of the current ones: %+v", list)
	}
	broken := strings.Replace(file.Text, `"ca"`, `"cipher":"x","ca_gone"`, 1)
	if _, err = target.InspectBackup(BackupText{Text: broken}); err == nil {
		t.Error("a backup with a broken endpoint is refused")
	}
}
