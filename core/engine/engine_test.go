package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/jsonrpc"
	"PorosCore/internal/logontask"
)

type event struct {
	method string
	params any
}

// tempDir is a temporary folder of a test, removed after it as t.TempDir does, but tried again for a few seconds.
// An antivirus that scans a file the test wrote may still hold it: Windows deletes such a file only once the last
// handle goes, and until then its folder is not empty, which t.TempDir reports as a failure at once.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "poros-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(10 * time.Second)
		for {
			err := os.RemoveAll(dir)
			if err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Errorf("the folder of the test stays: %v", err)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	return dir
}

func openTest(t *testing.T, dir string) (*Engine, *[]event) {
	t.Helper()
	events := &[]event{}
	e, err := Open(Config{DataDir: dir, Version: "0.1.0", Notify: func(method string, params any) {
		*events = append(*events, event{method, params})
	}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return e, events
}

func reason(err error) string {
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeFailed {
		return ""
	}
	switch data := rpcErr.Data.(type) {
	case map[string]string:
		return data["reason"]
	case RuleProblem:
		return data.Reason
	case OTPFailure:
		return data.Reason
	}
	return ""
}

func TestSettingsStartFromDefaultsAndPersist(t *testing.T) {
	dir := tempDir(t)
	e, events := openTest(t, dir)

	got, err := e.Settings()
	if err != nil || got != DefaultSettings() {
		t.Fatalf("a new folder must start from the defaults, got %+v, %v", got, err)
	}
	if got.Tun || got.SystemProxy {
		t.Fatal("TUN and System must both start off: the first Start asks")
	}

	want := DefaultSettings()
	want.Tun, want.SystemProxy, want.Preset, want.Language, want.SubscriptionUpdateHours = true, true, "hardened", "ru", 24
	if _, err = e.SetSettings(want); err != nil {
		t.Fatalf("SetSettings: %v", err)
	}
	if len(*events) != 1 || (*events)[0].method != EventSettingsChanged || (*events)[0].params != want {
		t.Fatalf("SetSettings must announce the new settings, got %+v", *events)
	}

	e.Close()
	reopened, _ := openTest(t, dir)
	if got, _ = reopened.Settings(); got != want {
		t.Fatalf("settings did not survive a restart: %+v", got)
	}
}

func TestSettingsRejectUnknownValues(t *testing.T) {
	e, events := openTest(t, tempDir(t))
	for name, change := range map[string]func(*Settings){
		"ui mode":          func(s *Settings) { s.UIMode = "expert" },
		"language":         func(s *Settings) { s.Language = "de" },
		"preset":           func(s *Settings) { s.Preset = "nope" },
		"negative hours":   func(s *Settings) { s.SubscriptionUpdateHours = -1 },
		"more than weekly": func(s *Settings) { s.SubscriptionUpdateHours = maxUpdateHours + 1 },
	} {
		settings := DefaultSettings()
		change(&settings)
		if _, err := e.SetSettings(settings); reason(err) == "" {
			t.Errorf("%s: expected a failed request, got %v", name, err)
		}
	}
	if len(*events) != 0 {
		t.Fatalf("a rejected change must not be announced: %+v", *events)
	}
}

func TestBuiltinPresets(t *testing.T) {
	e, _ := openTest(t, tempDir(t))
	presets, err := e.Presets()
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(presets))
	for i, p := range presets {
		ids[i] = p.ID
		if !p.Builtin || p.Name == "" {
			t.Errorf("preset %+v must be builtin and named", p)
		}
	}
	if !slices.Equal(ids, []string{"all", "hardened"}) {
		t.Fatalf("builtin presets: %v", ids)
	}
}

func TestOneEnginePerDataFolder(t *testing.T) {
	dir := tempDir(t)
	openTest(t, dir)
	started := time.Now()
	if second, err := Open(Config{DataDir: dir}); err == nil {
		second.Close()
		t.Fatal("a second engine on the same folder must not open")
	}
	if waited := time.Since(started); waited > 5*time.Second {
		t.Fatalf("the second Open waited %v instead of giving up", waited)
	}
}

func TestHandlersDecodeParamsStrictly(t *testing.T) {
	e, _ := openTest(t, tempDir(t))
	_, err := e.Handlers()[MethodSubscriptionsAdd](t.Context(), json.RawMessage(`{"link":"https://a.example"}`))
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("an unknown field must be rejected, got %v", err)
	}
}

// Earlier versions stored rights at sign-in: the setting goes from the data, the other settings stay with their
// secrets sealed, and a task of the setting that may be there is marked for removal.
func TestRightsAtSignInGoFromStoredSettings(t *testing.T) {
	for _, on := range []bool{true, false} {
		dir := tempDir(t)
		e, _ := openTest(t, dir)
		want := DefaultSettings()
		want.ProxyAuth, want.ProxyUsername, want.ProxyPassword, want.TextSize = true, "user", "secret-of-proxy", "large"
		if _, err := e.SetSettings(want); err != nil {
			t.Fatal(err)
		}
		err := e.db.Update(func(tx *bbolt.Tx) error {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(tx.Bucket(bucketSettings).Get(keySettings), &fields); err != nil {
				return err
			}
			fields["adminAutostart"] = json.RawMessage(strconv.FormatBool(on))
			data, err := json.Marshal(fields)
			if err != nil {
				return err
			}
			return tx.Bucket(bucketSettings).Put(keySettings, data)
		})
		if err != nil {
			t.Fatal(err)
		}
		e.Close()

		reopened, _ := openTest(t, dir)
		if got, err := reopened.Settings(); err != nil || got != want {
			t.Fatalf("adminAutostart %t: the settings after the migration %+v, %v", on, got, err)
		}
		err = reopened.db.View(func(tx *bbolt.Tx) error {
			data := tx.Bucket(bucketSettings).Get(keySettings)
			if bytes.Contains(data, []byte("adminAutostart")) || bytes.Contains(data, []byte("secret-of-proxy")) {
				t.Errorf("adminAutostart %t: stored %s", on, data)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if logontask.RemovalMarked(dir) != on {
			t.Errorf("adminAutostart %t: the task of the setting is marked for removal: %t", on, !on)
		}
	}
}
