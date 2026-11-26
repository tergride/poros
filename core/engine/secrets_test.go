package engine

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/secrets"
	"PorosCore/profile"
)

const (
	mineSecret     = "mine-secret-7f2c"
	subSecret      = "sub-secret-55e0"
	customPassword = "custom-secret-4b1e"
	endpointSecret = "endpoint-secret-0c9d"
	otpSecret      = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	proxySecret    = "proxy-secret-91ad"
	warpKey        = "yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk="
)

// filled is a data folder with each kind of record that holds secrets.
type filled struct {
	provider *provider
	sub      string
	mine     string
	server   string
	custom   string
	endpoint string
	key      string
	otp      string
}

func fillSecrets(t *testing.T, e *Engine) filled {
	t.Helper()
	f := filled{provider: newProvider(t)}
	f.provider.serve(http.StatusOK, "trojan://"+subSecret+"@sub.example.com:443?security=tls&sni=sub.example.com#Sub")
	update, err := e.AddSubscription(t.Context(), AddSubscription{URL: f.provider.url})
	if err != nil {
		t.Fatal(err)
	}
	f.sub = update.Group.ID
	added, err := e.AddServers(AddServers{NewGroup: "Mine", Text: "trojan://" + mineSecret + "@mine.example.com:443?security=tls&sni=mine.example.com#Mine"})
	if err != nil {
		t.Fatal(err)
	}
	f.mine, f.server = added.Group.ID, added.IDs[0]
	custom, err := e.SaveServer(SaveServer{GroupID: f.mine, Profile: profile.Profile{Type: profile.TypeCustom, Name: "Custom", Custom: &profile.Custom{
		JSON: json.RawMessage(`{"type":"trojan","server":"c.example.com","server_port":443,"password":"` + customPassword + `"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	f.custom = custom.ID
	entry := testEndpoint(t, "Office")
	entry.On, entry.Profile.OpenVPN.Password, f.key = true, endpointSecret, entry.Profile.OpenVPN.Key
	endpoint, err := e.SaveEndpoint(entry)
	if err != nil {
		t.Fatal(err)
	}
	f.endpoint = endpoint.ID
	code, err := e.SaveOTP(OTPEntry{Name: "Office", Secret: otpSecret, Kind: OTPKindTOTP, Algorithm: OTPSHA1, Digits: 6, Period: 30})
	if err != nil {
		t.Fatal(err)
	}
	f.otp = code.ID
	settings, err := e.Settings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Server = f.server
	settings.ProxyAuth, settings.ProxyUsername, settings.ProxyPassword = true, "me", proxySecret
	settings.WARP, settings.WARPWireGuard = true, WARPDevice{Endpoint: "162.159.192.1:2408", PrivateKey: warpKey,
		PeerKey: "xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=", Addresses: "172.16.0.2/32"}
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	return f
}

// changeData opens the data file of a closed engine as it lies on the disk.
func changeData(t *testing.T, dir string, change func(tx *bbolt.Tx) error) {
	t.Helper()
	db, err := bbolt.Open(filepath.Join(dir, DataFile), 0o600, &bbolt.Options{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Update(change); err != nil {
		t.Fatal(err)
	}
}

// rawData is every value of the data file of a closed engine.
func rawData(t *testing.T, dir string) string {
	t.Helper()
	var all bytes.Buffer
	changeData(t, dir, func(tx *bbolt.Tx) error {
		return tx.ForEach(func(_ []byte, bucket *bbolt.Bucket) error {
			return bucket.ForEach(func(_, value []byte) error {
				all.Write(value)
				all.WriteByte('\n')
				return nil
			})
		})
	})
	return all.String()
}

func reopenSecretsTest(t *testing.T, dir string) (*Engine, chan ConnectionState) {
	t.Helper()
	states := make(chan ConnectionState, 64)
	e, err := Open(Config{DataDir: dir, MixedPort: freePort(t), Notify: func(method string, params any) {
		if method == EventConnectionChanged {
			states <- params.(ConnectionState)
		}
	}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return e, states
}

// The data file keeps every secret sealed and the rest of its records open, and the core gives them open.
func TestSecretsAreSealedInTheDataFile(t *testing.T) {
	dir := tempDir(t)
	e, _ := openTest(t, dir)
	f := fillSecrets(t, e)
	e.Close()

	data := rawData(t, dir)
	for _, secret := range []string{mineSecret, subSecret, customPassword, endpointSecret, "PRIVATE KEY", otpSecret, proxySecret, warpKey, "secret-token"} {
		if strings.Contains(data, secret) {
			t.Errorf("%s lies open in the data file", secret)
		}
	}
	for _, open := range []string{secrets.Prefix, "mine.example.com", `"name":"Office"`, `"proxyUsername":"me"`} {
		if !strings.Contains(data, open) {
			t.Errorf("%s is not in the data file", open)
		}
	}

	e, _ = reopenSecretsTest(t, dir)
	if server, err := e.Server(ByID{ID: f.server}); err != nil || server.Profile.Trojan.Password != mineSecret || len(server.Server.Need) != 0 {
		t.Errorf("a server: %+v, %v", server, err)
	}
	if server, err := e.Server(ByID{ID: f.custom}); err != nil || !strings.Contains(string(server.Profile.Custom.JSON), customPassword) {
		t.Errorf("a custom outbound: %+v, %v", server, err)
	}
	groups, err := e.Groups()
	if err != nil || groups[0].ID != f.sub || groups[0].URL != f.provider.url || len(groups[0].Need) != 0 {
		t.Errorf("a subscription: %+v, %v", groups, err)
	}
	if entry, err := e.EndpointEntry(f.endpoint); err != nil || entry.Profile.OpenVPN.Password != endpointSecret || entry.Profile.OpenVPN.Key != f.key {
		t.Errorf("an endpoint: %+v, %v", entry, err)
	}
	if entry, err := e.OTPEntry(f.otp); err != nil || entry.Secret != otpSecret || entry.Lost {
		t.Errorf("a code of OTP: %+v, %v", entry, err)
	}
	if settings, err := e.Settings(); err != nil || settings.ProxyPassword != proxySecret || settings.WARPWireGuard.PrivateKey != warpKey {
		t.Errorf("the settings: %+v, %v", settings, err)
	}

	// A backup stays as it was: it opens on another computer.
	backup, err := e.CreateBackup(BackupParts{Parts: backupParts})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{mineSecret, subSecret, endpointSecret, otpSecret, proxySecret, warpKey, f.provider.url} {
		if !strings.Contains(backup.Text, secret) {
			t.Errorf("%s is not in the backup", secret)
		}
	}
	if strings.Contains(backup.Text, secrets.Prefix) {
		t.Error("a backup holds sealed values")
	}
}

// A data file written before the secrets were sealed reads as it is, and a save seals what it writes.
func TestPlainDataIsRead(t *testing.T) {
	dir := tempDir(t)
	e, _ := openTest(t, dir)
	e.Close()
	changeData(t, dir, func(tx *bbolt.Tx) error {
		if err := tx.DeleteBucket(bucketMeta); err != nil {
			return err
		}
		group := groupRecord{ID: "old", Kind: GroupManual, Name: "Old", AddedAt: 1}
		servers := []serverRecord{{ID: "s1", Profile: profile.Profile{Type: profile.TypeTrojan, Name: "Old", Server: "old.example.com", ServerPort: 443,
			Trojan: &profile.Trojan{Password: mineSecret}}}}
		settings := DefaultSettings()
		settings.ProxyAuth, settings.ProxyUsername, settings.ProxyPassword = true, "me", proxySecret
		otp := []OTPEntry{{ID: "o1", Name: "Office", Secret: otpSecret, Kind: OTPKindTOTP, Algorithm: OTPSHA1, Digits: 6, Period: 30}}
		return errorsOf(storeGroup(tx, &group, servers), putJSON(tx, bucketSettings, keySettings, settings), putJSON(tx, bucketOTP, keyOTP, otp))
	})
	if data := rawData(t, dir); !strings.Contains(data, mineSecret) || !strings.Contains(data, proxySecret) {
		t.Fatal("the old data file is not plain")
	}

	e, _ = reopenSecretsTest(t, dir)
	server, err := e.Server(ByID{ID: "s1"})
	if err != nil || server.Profile.Trojan.Password != mineSecret || len(server.Server.Need) != 0 {
		t.Fatalf("an old server: %+v, %v", server, err)
	}
	settings, err := e.Settings()
	if err != nil || settings.ProxyPassword != proxySecret {
		t.Fatalf("old settings: %+v, %v", settings, err)
	}
	if entry, err := e.OTPEntry("o1"); err != nil || entry.Secret != otpSecret {
		t.Fatalf("an old code of OTP: %+v, %v", entry, err)
	}
	if _, err = e.SaveServer(SaveServer{ID: "s1", Profile: server.Profile}); err != nil {
		t.Fatal(err)
	}
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	e.Close()
	if data := rawData(t, dir); strings.Contains(data, mineSecret) || strings.Contains(data, proxySecret) || !strings.Contains(data, otpSecret) {
		t.Error("a save leaves its secrets open, or what was not saved changed")
	}
}

func errorsOf(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// A data folder brought from another computer: its data key does not open, so its secrets are lost and their
// records wait for them, the start and the updates refuse with secrets_lost, and what is typed in again works.
func TestSecretsOfAnotherComputer(t *testing.T) {
	dir := tempDir(t)
	e, _ := openTest(t, dir)
	f := fillSecrets(t, e)
	e.Close()
	changeData(t, dir, func(tx *bbolt.Tx) error {
		// A key DPAPI sealed on another computer: this one cannot open it.
		return tx.Bucket(bucketMeta).Put(keyDataKey, append([]byte{1}, bytes.Repeat([]byte{0x5a}, 230)...))
	})
	requests := f.provider.requests()

	e, states := reopenSecretsTest(t, dir)
	servers, err := e.Servers(ServersQuery{GroupID: f.mine})
	if err != nil || len(servers) != 2 || !slices.Equal(servers[0].Need, []string{"trojan.password"}) || !slices.Equal(servers[1].Need, []string{customSecret}) {
		t.Fatalf("the servers wait for their secrets: %+v, %v", servers, err)
	}
	if server, err := e.Server(ByID{ID: f.server}); err != nil || server.Profile.Trojan.Password != "" || server.Profile.Server != "mine.example.com" {
		t.Errorf("a lost secret is empty, the rest stays: %+v, %v", server, err)
	}
	groups, err := e.Groups()
	if err != nil || groups[0].URL != "" || !slices.Equal(groups[0].Need, []string{"url"}) {
		t.Fatalf("the subscription waits for its link: %+v, %v", groups, err)
	}
	if _, err := e.UpdateSubscription(t.Context(), f.sub); reason(err) != ReasonSecretsLost || f.provider.requests() != requests {
		t.Errorf("an update without the link: %v", err)
	}
	endpoints, err := e.Endpoints()
	if err != nil || !slices.Equal(endpoints[0].Need, []string{SignInPassword, SignInCert, SignInKey}) {
		t.Fatalf("the endpoint waits for its data: %+v, %v", endpoints, err)
	}
	codes, err := e.OTPCodes()
	if err != nil || !codes[0].Lost || codes[0].Code != "" {
		t.Errorf("a code of OTP without its secret: %+v, %v", codes, err)
	}
	if _, err := e.OTPCodeNow(f.otp); reason(err) != ReasonSecretsLost {
		t.Errorf("a code now: %v", err)
	}
	if _, err := e.OTPLink(f.otp); reason(err) != ReasonSecretsLost {
		t.Errorf("a link of the code: %v", err)
	}
	if _, err := e.ShareLinks(ServerIDs{IDs: []string{f.server}}); reason(err) != ReasonSecretsLost {
		t.Errorf("a share link without the secret: %v", err)
	}
	if _, err := e.ExportServer(ByID{ID: f.custom}); reason(err) != ReasonSecretsLost {
		t.Errorf("an export without the secret: %v", err)
	}

	start := func(want string) {
		t.Helper()
		e.StartConnection()
		if state := waitState(t, states); state.State != ConnectionIdle || state.Error != ReasonSecretsLost || !strings.Contains(state.ErrorText, want) {
			t.Fatalf("a start without the secrets of %s: %+v", want, state)
		}
	}
	start("local proxy")
	settings, err := e.Settings()
	if err != nil || settings.ProxyPassword != "" || settings.WARPWireGuard.PrivateKey != "" {
		t.Fatalf("the settings lost their secrets: %+v, %v", settings, err)
	}
	settings.ProxyPassword = proxySecret
	if _, err = e.SetSettings(settings); reason(err) != ReasonInvalidValue {
		t.Fatalf("WARP stays on without its key: %v", err)
	}
	settings.WARP = false
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	start("Office")
	entry, err := e.EndpointEntry(f.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	entry.On = false
	if _, err = e.SaveEndpoint(entry); err != nil {
		t.Fatal(err)
	}
	start("Mine")

	// A copy of a server that waits keeps waiting, and a secret typed in again is no longer lost.
	subServers, err := e.Servers(ServersQuery{GroupID: f.sub})
	if err != nil || len(subServers) != 1 || !slices.Equal(subServers[0].Need, []string{"trojan.password"}) {
		t.Fatalf("the server of the subscription: %+v, %v", subServers, err)
	}
	moved, err := e.MoveServers(MoveServers{IDs: []string{subServers[0].ID}, GroupID: f.mine})
	if err != nil {
		t.Fatal(err)
	}
	if copied, err := e.Servers(ServersQuery{IDs: moved.IDs}); err != nil || len(copied) != 1 || !slices.Equal(copied[0].Need, []string{"trojan.password"}) {
		t.Errorf("a copy of a server that waits: %+v, %v", copied, err)
	}
	server, err := e.Server(ByID{ID: f.server})
	if err != nil {
		t.Fatal(err)
	}
	server.Profile.Trojan.Password = mineSecret
	if saved, err := e.SaveServer(SaveServer{ID: f.server, Profile: server.Profile}); err != nil || len(saved.Need) != 0 {
		t.Fatalf("a secret typed in again: %+v, %v", saved, err)
	}
	group, err := e.ConfigureGroup(ConfigureGroup{ID: f.sub, URL: f.provider.url, Settings: groups[0].Settings})
	if err != nil || len(group.Need) != 0 {
		t.Fatalf("a link typed in again: %+v, %v", group, err)
	}
	if _, err = e.UpdateSubscription(t.Context(), f.sub); err != nil || f.provider.requests() != requests+1 {
		t.Fatalf("an update with the link: %v", err)
	}
	code, err := e.OTPEntry(f.otp)
	if err != nil {
		t.Fatal(err)
	}
	code.Secret = otpSecret
	if saved, err := e.SaveOTP(code); err != nil || saved.Lost {
		t.Fatalf("a secret of OTP typed in again: %+v, %v", saved, err)
	}
	if codes, err := e.OTPCodes(); err != nil || codes[0].Lost || codes[0].Code == "" {
		t.Errorf("a code of OTP with its secret: %+v, %v", codes, err)
	}

	// The new data key of this computer opens what was saved under it.
	e.Close()
	e, _ = reopenSecretsTest(t, dir)
	if server, err := e.Server(ByID{ID: f.server}); err != nil || server.Profile.Trojan.Password != mineSecret || len(server.Server.Need) != 0 {
		t.Errorf("a secret typed in again, after a restart: %+v, %v", server, err)
	}
	if groups, err := e.Groups(); err != nil || groups[0].URL != f.provider.url || len(groups[0].Need) != 0 {
		t.Errorf("a link typed in again, after a restart: %+v, %v", groups, err)
	}
}
