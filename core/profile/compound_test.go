package profile

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestValidateCompoundProfiles(t *testing.T) {
	valid := []Profile{
		{Type: TypeChain, Name: "Two hops", Chain: &Chain{Servers: []string{"a", "b"}}},
		{Type: TypeCustom, Custom: &Custom{JSON: json.RawMessage(`{"type": "tor"}`)}},
		{Type: TypeDirect, Direct: &Direct{}},
	}
	for _, p := range valid {
		if err := p.Validate(); err != nil {
			t.Errorf("%s: %v; the compound profiles need no address", p.Type, err)
		}
	}
	for name, tt := range map[string]struct {
		p    Profile
		want string
	}{
		"a chain of one":       {Profile{Type: TypeChain, Chain: &Chain{Servers: []string{"a"}}}, "at least two"},
		"a server twice":       {Profile{Type: TypeChain, Chain: &Chain{Servers: []string{"a", "a"}}}, "twice"},
		"custom not an object": {Profile{Type: TypeCustom, Custom: &Custom{JSON: json.RawMessage(`[1]`)}}, "JSON object"},
		"custom without type":  {Profile{Type: TypeCustom, Custom: &Custom{JSON: json.RawMessage(`{"server": "x"}`)}}, "no type"},
		"custom direct":        {Profile{Type: TypeCustom, Custom: &Custom{JSON: json.RawMessage(`{"type": "direct"}`)}}, "no server"},
		"custom with a detour": {Profile{Type: TypeCustom, Custom: &Custom{JSON: json.RawMessage(`{"type": "tor", "detour": "x"}`)}}, "chain"},
	} {
		if err := tt.p.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v, want %q", name, err, tt.want)
		}
	}

	chain, _ := json.Marshal(valid[0])
	if _, err := ParseLink("poros://add/" + base64.RawURLEncoding.EncodeToString(chain)); !errors.Is(err, ErrUnsupported) {
		t.Errorf("a chain names servers of one Poros and cannot come in a link: %v", err)
	}
	direct := ShareLink(valid[2])
	if p, err := ParseLink(direct); err != nil || p.Type != TypeDirect {
		t.Errorf("Direct as a link: %s reads as %+v, %v", direct, p, err)
	}
}
