package engine

import (
	"encoding/json"
	"log"
	"slices"
	"strings"
	"sync"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/secrets"
	"PorosCore/profile"
)

var (
	bucketMeta = []byte("meta")
	keyDataKey = []byte("dataKey")
)

// boxes are the data keys of the open data files by their database: getJSON and putJSON have the transaction alone.
var boxes sync.Map

func boxOf(tx *bbolt.Tx) *secrets.Box {
	if box, ok := boxes.Load(tx.DB()); ok {
		return box.(*secrets.Box)
	}
	return nil
}

// openDataKey opens the data key of the data file, or makes the first one. A key this computer cannot open, as one
// the folder brought from another computer, gives way to a new one, and the secrets sealed under it are lost here.
func openDataKey(db *bbolt.DB) (*secrets.Box, error) {
	var box *secrets.Box
	err := db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(bucketMeta)
		if err != nil {
			return err
		}
		if stored := bucket.Get(keyDataKey); stored != nil {
			key, err := secrets.Unprotect(stored)
			if err == nil {
				defer clear(key)
				box, err = secrets.New(key)
				return err
			}
			log.Printf("secrets: the data key is of another computer (%v), so the secrets saved there stay unread", err)
		}
		key, err := secrets.NewKey()
		if err != nil {
			return err
		}
		defer clear(key)
		sealed, err := secrets.Protect(key)
		if err != nil {
			return err
		}
		if err := bucket.Put(keyDataKey, sealed); err != nil {
			return err
		}
		box, err = secrets.New(key)
		return err
	})
	return box, err
}

const customSecret = "custom.json"

// sealProfile seals the secrets of a profile in place; the caller gives a copy.
func sealProfile(box *secrets.Box, p *profile.Profile) {
	for _, s := range p.Secrets() {
		*s.Value = box.Seal(*s.Value)
	}
	if text := customText(*p); text != "" && !sealedRaw(p.Custom.JSON) {
		sealed, _ := json.Marshal(box.Seal(text))
		p.Custom = &profile.Custom{JSON: sealed}
	}
}

// customText is the JSON of a custom outbound, "" for none: a lost one is kept as null.
func customText(p profile.Profile) string {
	if p.Custom == nil {
		return ""
	}
	if text := strings.TrimSpace(string(p.Custom.JSON)); text != "null" {
		return text
	}
	return ""
}

// sealedRaw tells the JSON of a custom outbound that is a sealed value: a JSON string with the prefix.
func sealedRaw(raw json.RawMessage) bool {
	var text string
	return json.Unmarshal(raw, &text) == nil && secrets.Sealed(text)
}

// openProfile opens the secrets of a profile in place and names those it could not, which become empty.
func openProfile(box *secrets.Box, p *profile.Profile) (lost []string) {
	for _, s := range p.Secrets() {
		value, err := box.Open(*s.Value)
		if err != nil {
			lost = append(lost, s.Name)
		}
		*s.Value = value
	}
	if c := p.Custom; c != nil && sealedRaw(c.JSON) {
		var text string
		_ = json.Unmarshal(c.JSON, &text)
		value, err := box.Open(text)
		if err != nil {
			lost = append(lost, customSecret)
			c.JSON = nil
		} else {
			c.JSON = json.RawMessage(value)
		}
	}
	return lost
}

// stillLost keeps the names of lost secrets that are still empty: one the user typed in again is no longer lost.
func stillLost(p profile.Profile, lost []string) []string {
	if len(lost) == 0 {
		return nil
	}
	values := map[string]string{}
	for _, s := range p.Secrets() {
		values[s.Name] = *s.Value
	}
	values[customSecret] = customText(p)
	var kept []string
	for _, name := range lost {
		if values[name] == "" && !slices.Contains(kept, name) {
			kept = append(kept, name)
		}
	}
	return kept
}

// cloneJSON copies a value through its JSON, as the data file keeps it: the sealing of a copy leaves the caller's
// value open.
func cloneJSON[T any](v T) (T, error) {
	var out T
	data, err := json.Marshal(v)
	if err == nil {
		err = json.Unmarshal(data, &out)
	}
	return out, err
}

// sealValue gives a sealed copy of a value of a bucket that holds secrets; any other value comes back as it is.
func sealValue(box *secrets.Box, v any) (any, error) {
	switch v := v.(type) {
	case []serverRecord:
		list, err := cloneJSON(v)
		for i := range list {
			list[i].Lost = stillLost(list[i].Profile, list[i].Lost)
			sealProfile(box, &list[i].Profile)
		}
		return list, err
	case groupRecord:
		return sealGroup(box, v), nil
	case *groupRecord:
		rec := sealGroup(box, *v)
		return &rec, nil
	case []endpointRecord:
		list, err := cloneJSON(v)
		for i := range list {
			sealProfile(box, &list[i].Profile)
		}
		return list, err
	case []OTPEntry:
		list := slices.Clone(v)
		for i := range list {
			list[i].Lost = list[i].Lost && list[i].Secret == ""
			list[i].Secret = box.Seal(list[i].Secret)
		}
		return list, nil
	case Settings:
		return sealSettings(box, v), nil
	case *Settings:
		s := sealSettings(box, *v)
		return &s, nil
	}
	return v, nil
}

func sealGroup(box *secrets.Box, rec groupRecord) groupRecord {
	if rec.URL != "" {
		rec.Lost = nil
	}
	rec.URL = box.Seal(rec.URL)
	return rec
}

func sealSettings(box *secrets.Box, s Settings) Settings {
	s.ProxyPassword = box.Seal(s.ProxyPassword)
	s.WARPWireGuard.PrivateKey = box.Seal(s.WARPWireGuard.PrivateKey)
	s.WARPMASQUE.PrivateKey = box.Seal(s.WARPMASQUE.PrivateKey)
	return s
}

// openValue opens in place what getJSON decoded from a bucket that holds secrets. A secret that does not open becomes
// empty, and its record says it is lost: servers and subscriptions by name, endpoints among the data they need,
// codes of OTP by a mark. The settings have no such mark: the secrets they lack are told by their emptiness.
func openValue(box *secrets.Box, v any) {
	open := func(value string) (string, bool) {
		opened, err := box.Open(value)
		return opened, err == nil
	}
	switch v := v.(type) {
	case *[]serverRecord:
		for i := range *v {
			rec := &(*v)[i]
			lost := openProfile(box, &rec.Profile)
			rec.Lost = stillLost(rec.Profile, append(rec.Lost, lost...))
		}
	case *groupRecord:
		var ok bool
		if v.URL, ok = open(v.URL); !ok {
			v.Lost = []string{"url"}
		}
	case *[]endpointRecord:
		for i := range *v {
			rec := &(*v)[i]
			for _, name := range openProfile(box, &rec.Profile) {
				if need := signInOf(name); need != "" && !slices.Contains(rec.Need, need) {
					rec.Need = append(rec.Need, need)
				}
			}
			slices.SortStableFunc(rec.Need, func(a, b string) int { return slices.Index(signInOrder, a) - slices.Index(signInOrder, b) })
		}
	case *[]OTPEntry:
		for i := range *v {
			var ok bool
			if (*v)[i].Secret, ok = open((*v)[i].Secret); !ok {
				(*v)[i].Lost = true
			}
		}
	case *Settings:
		v.ProxyPassword, _ = open(v.ProxyPassword)
		v.WARPWireGuard.PrivateKey, _ = open(v.WARPWireGuard.PrivateKey)
		v.WARPMASQUE.PrivateKey, _ = open(v.WARPMASQUE.PrivateKey)
	}
}

// checkSecrets refuses a start with secrets of the settings or of an endpoint that is on that were saved on another
// computer. Only that leaves them empty: the settings and an endpoint that is on cannot be saved so. The servers of
// the connection are checked as they are resolved.
func (e *Engine) checkSecrets(s Settings) error {
	if s.ProxyAuth && s.ProxyPassword == "" {
		return failure(ReasonSecretsLost, "the password of the local proxy was saved on another computer")
	}
	device := s.WARPWireGuard
	if s.WARPMode == WARPModeMASQUE {
		device = s.WARPMASQUE
	}
	if s.WARP && device.PrivateKey == "" {
		return failure(ReasonSecretsLost, "the key of the WARP device was saved on another computer: register the device again")
	}
	var records []endpointRecord
	err := e.db.View(func(tx *bbolt.Tx) (err error) {
		records, err = loadEndpoints(tx)
		return err
	})
	if err != nil {
		return err
	}
	for _, r := range records {
		if r.On && len(r.Need) > 0 {
			return failure(ReasonSecretsLost, "the endpoint %s was saved on another computer and needs %s again", r.Profile.Name, strings.Join(r.Need, ", "))
		}
	}
	return nil
}

// signInOf names a lost secret of an endpoint as the data it needs: password, cert, key, keyPassword, wrapKey.
func signInOf(secret string) string {
	_, field, _ := strings.Cut(secret, ".")
	switch field {
	case "password":
		return SignInPassword
	case "cert":
		return SignInCert
	case "key":
		return SignInKey
	case "key_password":
		return SignInKeyPassword
	case "control_wrap.key":
		return SignInWrapKey
	}
	return ""
}
