package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

// backupSource is an engine with a group of the user's, a preset, a code of OTP and settings of its own.
func backupSource(t *testing.T) (*Engine, string, string, string) {
	t.Helper()
	e, _ := openTest(t, t.TempDir())
	added, err := e.AddServers(AddServers{NewGroup: "Mine", Text: "trojan://pw@trojan.example.com:443?sni=example.com#Home"})
	if err != nil {
		t.Fatal(err)
	}
	rule := PresetRule{Conditions: []RuleCondition{{Kind: CondSuffix, Values: []string{"example.org"}}}, Action: ActionDirect}
	preset, err := e.SavePreset(PresetCopy{BasedOn: "all", Name: "Home", Rules: []PresetRule{rule}})
	if err != nil {
		t.Fatal(err)
	}
	code, err := e.SaveOTP(totp("GitHub", "JBSWY3DPEHPK3PXP"))
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := e.Settings()
	settings.Server, settings.Preset, settings.LogLines = added.IDs[0], preset.ID, 700
	if _, err = e.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	return e, added.IDs[0], preset.ID, code.ID
}

func TestBackupThereAndBack(t *testing.T) {
	source, serverID, presetID, codeID := backupSource(t)
	file, err := source.CreateBackup(BackupParts{Parts: []string{BackupOTP, BackupProfiles, BackupSettings, BackupRouting}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(file.Name, "poros-backup-") || !strings.HasSuffix(file.Name, ".json") {
		t.Errorf("name: %s", file.Name)
	}

	target, events := openTest(t, t.TempDir())
	info, err := target.InspectBackup(BackupText{Text: file.Text})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(info.Parts, ",") != "profiles,routing,settings,otp" || info.Groups != 1 || info.Servers != 1 || info.Presets != 1 || info.Codes != 1 || info.Created == 0 {
		t.Errorf("info: %+v", info)
	}
	if _, err = target.RestoreBackup(BackupRestore{Text: file.Text}); err != nil {
		t.Fatal(err)
	}
	if p, found, _ := target.serverProfile(serverID); !found || p.Server != "trojan.example.com" {
		t.Errorf("the server keeps its ID: %+v %t", p, found)
	}
	if groups, _ := target.Groups(); len(groups) != 1 || groups[0].Name != "Mine" || groups[0].Servers != 1 {
		t.Errorf("groups: %+v", groups)
	}
	if copy, err := target.PresetCopy(presetID); err != nil || copy.Name != "Home" || len(copy.Rules) != 1 {
		t.Errorf("preset: %+v %v", copy, err)
	}
	if entry, err := target.OTPEntry(codeID); err != nil || entry.Name != "GitHub" {
		t.Errorf("code: %+v %v", entry, err)
	}
	if settings, _ := target.Settings(); settings.Server != serverID || settings.Preset != presetID || settings.LogLines != 700 {
		t.Errorf("settings: %s %s %d", settings.Server, settings.Preset, settings.LogLines)
	}
	seen := map[string]bool{}
	for _, ev := range *events {
		seen[ev.method] = true
	}
	for _, method := range []string{EventGroupsChanged, EventServersChanged, EventPresetsChanged, EventSettingsChanged, EventOTPChanged} {
		if !seen[method] {
			t.Errorf("no %s after the restore", method)
		}
	}
}

func TestBackupRestoresOnlyTheChosenParts(t *testing.T) {
	source, _, _, _ := backupSource(t)
	file, err := source.CreateBackup(BackupParts{Parts: []string{BackupProfiles, BackupRouting, BackupSettings, BackupOTP}})
	if err != nil {
		t.Fatal(err)
	}
	target, _ := openTest(t, t.TempDir())
	own, _ := target.SaveOTP(totp("Mail", "KRSXG5BAMFZXG5BA"))
	if _, err = target.RestoreBackup(BackupRestore{Text: file.Text, Parts: []string{BackupProfiles}}); err != nil {
		t.Fatal(err)
	}
	if codes, _ := target.OTPCodes(); len(codes) != 1 || codes[0].ID != own.ID {
		t.Errorf("the codes stay: %+v", codes)
	}
	if presets, _ := target.Presets(); len(presets) != 2 {
		t.Errorf("only the built-in presets: %+v", presets)
	}
	// The settings stayed, and their server, which the backup did not choose here, is none.
	if settings, _ := target.Settings(); settings.LogLines != DefaultSettings().LogLines || settings.Server != "" {
		t.Errorf("settings: %d %q", settings.LogLines, settings.Server)
	}
	// Settings without their preset take the standard one.
	if _, err = target.RestoreBackup(BackupRestore{Text: file.Text, Parts: []string{BackupSettings}}); err != nil {
		t.Fatal(err)
	}
	if settings, _ := target.Settings(); settings.Preset != standardPreset || settings.LogLines != 700 {
		t.Errorf("settings without their preset: %s %d", settings.Preset, settings.LogLines)
	}
	if _, err = target.RestoreBackup(BackupRestore{Text: file.Text, Parts: []string{"traffic"}}); reason(err) != ReasonInvalidBackup {
		t.Errorf("an unknown part: %v", err)
	}
}

func TestBackupWithAWrongPartChangesNothing(t *testing.T) {
	source, _, _, _ := backupSource(t)
	file, err := source.CreateBackup(BackupParts{Parts: []string{BackupProfiles, BackupOTP}})
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err = json.Unmarshal([]byte(file.Text), &raw); err != nil {
		t.Fatal(err)
	}
	for _, list := range raw["servers"].(map[string]any) {
		server := list.([]any)[0].(map[string]any)
		server["profile"].(map[string]any)["server"] = ""
	}
	broken, _ := json.Marshal(raw)

	target, _ := openTest(t, t.TempDir())
	own, _ := target.SaveOTP(totp("Mail", "KRSXG5BAMFZXG5BA"))
	for name, text := range map[string]string{
		"a server without its address": string(broken),
		"no JSON":                      "poros",
		"another kind":                 `{"kind":"poros-route","v":1,"parts":[]}`,
		"a later version":              `{"kind":"poros-backup","v":2,"parts":[]}`,
	} {
		if _, err := target.RestoreBackup(BackupRestore{Text: text}); reason(err) != ReasonInvalidBackup {
			t.Errorf("%s: %v", name, err)
		}
	}
	if codes, _ := target.OTPCodes(); len(codes) != 1 || codes[0].ID != own.ID {
		t.Errorf("nothing changed: %+v", codes)
	}
	if groups, _ := target.Groups(); len(groups) != 0 {
		t.Errorf("no groups came: %+v", groups)
	}
}
