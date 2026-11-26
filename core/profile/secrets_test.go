package profile

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Every secret field of the forms is a secret of the profile, and every secret names the place it lives at.
func TestSecretsCoverTheForms(t *testing.T) {
	for _, form := range Forms() {
		var p Profile
		if err := json.Unmarshal(fmt.Appendf(nil, `{"type":%q,%q:{}}`, form.Type, PartOf(form.Type)), &p); err != nil {
			t.Fatalf("%s: %v", form.Type, err)
		}
		names := map[string]bool{}
		for _, s := range p.Secrets() {
			names[s.Name] = true
			*s.Value = s.Name
		}
		for _, f := range form.Fields {
			if f.Kind == FieldSecret && !names[f.Path] {
				t.Errorf("%s: the field %s is no secret of the profile", form.Type, f.Path)
			}
		}
		data, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		var tree any
		if err = json.Unmarshal(data, &tree); err != nil {
			t.Fatal(err)
		}
		for name := range names {
			value := tree
			for key := range strings.SplitSeq(name, ".") {
				object, _ := value.(map[string]any)
				value = object[key]
			}
			if value != name {
				t.Errorf("%s: the secret %s lives elsewhere", form.Type, name)
			}
		}
	}
}
