package profile

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// typeAt follows a path of JSON keys through the stored profile.
func typeAt(path string) (reflect.Type, bool) {
	t := reflect.TypeFor[Profile]()
	for _, key := range strings.Split(path, ".") {
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return nil, false
		}
		found := false
		for i := range t.NumField() {
			if name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ","); name == key {
				t, found = t.Field(i).Type, true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t, true
}

var fieldTypes = map[string]reflect.Type{
	FieldText:     reflect.TypeFor[string](),
	FieldSecret:   reflect.TypeFor[string](),
	FieldLines:    reflect.TypeFor[string](),
	FieldNumber:   reflect.TypeFor[int](),
	FieldBool:     reflect.TypeFor[bool](),
	FieldList:     reflect.TypeFor[[]string](),
	FieldServers:  reflect.TypeFor[[]string](),
	FieldJSON:     reflect.TypeFor[json.RawMessage](),
	FieldHeaders:  reflect.TypeFor[map[string]string](),
	FieldBytes:    reflect.TypeFor[[]int](),
	FieldUTLS:     reflect.TypeFor[UTLS](),
	FieldSecurity: reflect.TypeFor[TLS](),
}

// Every field of every form lives where it says, as the kind of value its editor writes, and depends only on
// fields of the same profile.
func TestFormsMatchTheProfile(t *testing.T) {
	tabs := []string{TabCommon, TabTransport, TabSecurity, TabMux, TabAdvanced}
	types := map[string]bool{}
	for _, form := range Forms() {
		types[form.Type] = true
		own := func(path string) bool {
			return path == "name" || path == "server" || path == "server_port" || strings.HasPrefix(path, form.Type+".")
		}
		for _, f := range form.Fields {
			got, ok := typeAt(f.Path)
			want := fieldTypes[f.Kind]
			if f.Kind == FieldSelect {
				want = reflect.TypeFor[string]()
				if f.Numeric {
					want = reflect.TypeFor[int]()
				}
			}
			switch {
			case !ok || !own(f.Path):
				t.Errorf("%s: %s is not in the profile", form.Type, f.Path)
			case got != want:
				t.Errorf("%s: %s is %v, and a %s field writes %v", form.Type, f.Path, got, f.Kind, want)
			case !slices.Contains(tabs, f.Tab):
				t.Errorf("%s: %s is on no tab", form.Type, f.Path)
			case f.Label == "":
				t.Errorf("%s: %s has no label", form.Type, f.Path)
			case (f.Kind == FieldSelect || f.Kind == FieldUTLS || f.Kind == FieldSecurity) && len(f.Options) == 0:
				t.Errorf("%s: %s has no options", form.Type, f.Path)
			}
			for _, c := range f.When {
				if _, ok := typeAt(c.Path); !ok || !own(c.Path) {
					t.Errorf("%s: %s depends on %s, which is not in the profile", form.Type, f.Path, c.Path)
				}
			}
		}
	}
	for _, choice := range NewProfiles() {
		if !types[choice.Profile.Type] {
			t.Errorf("«New profile» makes %s, which has no form", choice.ID)
		}
		if err := choice.Profile.validatePart(); err != nil {
			t.Errorf("«New profile» %s: %v", choice.ID, err)
		}
	}
}

// The choices of selects are those the profile takes.
func TestFormChoicesAreValid(t *testing.T) {
	forms := map[string]ProfileForm{}
	for _, form := range Forms() {
		forms[form.Type] = form
	}
	options := func(typ, path string) []string {
		for _, f := range forms[typ].Fields {
			if f.Path == path {
				return f.Options
			}
		}
		t.Fatalf("%s has no field %s", typ, path)
		return nil
	}
	ss := Profile{Type: TypeShadowsocks, Server: "ss.example.com", ServerPort: 8388}
	keys := map[string]string{
		"2022-blake3-aes-128-gcm":       "MDEyMzQ1Njc4OWFiY2RlZg==",
		"2022-blake3-aes-256-gcm":       "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		"2022-blake3-chacha20-poly1305": "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
	}
	for _, method := range options(TypeShadowsocks, "shadowsocks.method") {
		password := keys[method]
		if password == "" {
			password = "password"
		}
		ss.Shadowsocks = &Shadowsocks{Method: method, Password: password}
		if err := ss.Validate(); err != nil {
			t.Errorf("Shadowsocks %s: %v", method, err)
		}
	}
	ovpn := Profile{Type: TypeOpenVPN, Server: "vpn.example.com", ServerPort: 1194}
	for _, network := range options(TypeOpenVPN, "openvpn.network") {
		ovpn.OpenVPN = &OpenVPN{Network: network, PeerFingerprint: []string{"AA:BB"}}
		if err := ovpn.Validate(); err != nil {
			t.Errorf("OpenVPN over %q: %v", network, err)
		}
	}
	snell := Profile{Type: TypeSnell, Server: "snell.example.com", ServerPort: 443}
	for _, version := range options(TypeSnell, "snell.version") {
		snell.Snell = &Snell{Version: int(version[0] - '0'), PSK: "a-long-test-psk"}
		if err := snell.Validate(); err != nil {
			t.Errorf("Snell v%s: %v", version, err)
		}
	}
}
