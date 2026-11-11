package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/jsonrpc"
)

const (
	backupKind    = "poros-backup"
	backupVersion = 1
	// stopWait bounds the stop of the connection before a restore.
	stopWait = 20 * time.Second
)

var backupParts = []string{BackupProfiles, BackupRouting, BackupSettings, BackupOTP}

// backupFile is a backup as written: the parts it has, each as the core stores it. The traffic history, the logs and
// the results of tests are not in it.
type backupFile struct {
	Kind     string                    `json:"kind"`
	V        int                       `json:"v"`
	Version  string                    `json:"version"`
	Created  int64                     `json:"created"`
	Parts    []string                  `json:"parts"`
	Groups   []groupRecord             `json:"groups,omitempty"`
	Servers  map[string][]serverRecord `json:"servers,omitempty"`
	Presets  []copyRecord              `json:"presets,omitempty"`
	Settings *Settings                 `json:"settings,omitempty"`
	OTP      []OTPEntry                `json:"otp,omitempty"`
}

func invalidBackup(format string, args ...any) error {
	return jsonrpc.Failed(ReasonInvalidBackup, format, args...)
}

func checkParts(parts []string) error {
	for _, p := range parts {
		if !slices.Contains(backupParts, p) {
			return invalidBackup("there is no part %q", p)
		}
	}
	return nil
}

// CreateBackup writes the parts asked for, in the order of backupParts.
func (e *Engine) CreateBackup(p BackupParts) (BackupFile, error) {
	if err := checkParts(p.Parts); err != nil {
		return BackupFile{}, err
	}
	if len(p.Parts) == 0 {
		return BackupFile{}, invalidBackup("a backup needs a part")
	}
	now := e.now()
	file := backupFile{Kind: backupKind, V: backupVersion, Version: e.UpdatesState().Current, Created: now.UnixMilli()}
	for _, part := range backupParts {
		if slices.Contains(p.Parts, part) {
			file.Parts = append(file.Parts, part)
		}
	}
	has := func(part string) bool { return slices.Contains(file.Parts, part) }
	err := e.db.View(func(tx *bbolt.Tx) error {
		if has(BackupProfiles) {
			store, err := loadServers(tx)
			if err != nil {
				return err
			}
			file.Groups, file.Servers = store.groups, store.servers
		}
		if has(BackupRouting) {
			file.Presets = []copyRecord{}
			if err := tx.Bucket(bucketPresets).ForEach(func(key, _ []byte) error {
				var record copyRecord
				_, err := getJSON(tx, bucketPresets, key, &record)
				file.Presets = append(file.Presets, record)
				return err
			}); err != nil {
				return err
			}
		}
		if has(BackupOTP) {
			entries, err := loadOTP(tx)
			file.OTP = nonNil(entries)
			return err
		}
		return nil
	})
	if err != nil {
		return BackupFile{}, err
	}
	if has(BackupSettings) {
		settings, err := e.Settings()
		if err != nil {
			return BackupFile{}, err
		}
		file.Settings = &settings
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return BackupFile{}, err
	}
	return BackupFile{Name: "poros-backup-" + now.Format("2006-01-02") + ".json", Text: string(data)}, nil
}

// readBackup reads a backup and checks the parts it has, each as its own save would: a backup with a part that
// cannot be taken is refused whole.
func (e *Engine) readBackup(text string) (backupFile, error) {
	var file backupFile
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &file); err != nil {
		return file, invalidBackup("the file is no JSON of a backup: %v", err)
	}
	if file.Kind != backupKind {
		return file, invalidBackup("the file is no backup of Poros")
	}
	if file.V != backupVersion {
		return file, invalidBackup("the backup is of version %d, and this Poros reads version %d", file.V, backupVersion)
	}
	if err := checkParts(file.Parts); err != nil {
		return file, err
	}
	has := func(part string) bool { return slices.Contains(file.Parts, part) }
	if has(BackupProfiles) {
		if err := checkBackupServers(file); err != nil {
			return file, err
		}
	}
	if has(BackupRouting) {
		if err := e.checkBackupPresets(file.Presets); err != nil {
			return file, err
		}
	}
	if has(BackupSettings) {
		if file.Settings == nil {
			return file, invalidBackup("the settings are missing")
		}
		// Whether the preset and the server are there is settled when the parts are in place.
		check := *file.Settings
		check.Preset, check.Server = standardPreset, ""
		if err := e.validateSettings(check); err != nil {
			return file, invalidBackup("settings: %v", err)
		}
	}
	if has(BackupOTP) {
		ids := map[string]bool{}
		for _, entry := range file.OTP {
			if entry.ID == "" || ids[entry.ID] {
				return file, invalidBackup("a code of OTP has no ID or the ID of another")
			}
			ids[entry.ID] = true
			if problem := entry.entry().Check(); problem != nil {
				return file, invalidBackup("OTP: %v", problem)
			}
		}
	}
	return file, nil
}

func checkBackupServers(file backupFile) error {
	groups, servers := map[string]bool{}, map[string]bool{}
	for _, g := range file.Groups {
		switch {
		case g.ID == "" || groups[g.ID]:
			return invalidBackup("a group has no ID or the ID of another")
		case g.Kind != GroupSubscription && g.Kind != GroupManual:
			return invalidBackup("the group %q is of no known kind", g.ID)
		case g.Kind == GroupSubscription && !strings.HasPrefix(g.URL, "http://") && !strings.HasPrefix(g.URL, "https://"):
			return invalidBackup("the subscription %q has no link", g.ID)
		}
		groups[g.ID] = true
	}
	for groupID, list := range file.Servers {
		if !groups[groupID] {
			return invalidBackup("servers of the group %q, which is not in the backup", groupID)
		}
		for _, s := range list {
			if s.ID == "" || servers[s.ID] {
				return invalidBackup("a server has no ID or the ID of another")
			}
			servers[s.ID] = true
			if err := s.Profile.Validate(); err != nil {
				return invalidBackup("the server %q: %v", s.Profile.Name, err)
			}
		}
	}
	return nil
}

func (e *Engine) checkBackupPresets(presets []copyRecord) error {
	ids, names := map[string]bool{}, map[string]bool{}
	for _, record := range presets {
		name := strings.ToLower(strings.TrimSpace(record.Name))
		switch {
		case record.ID == "" || ids[record.ID]:
			return invalidBackup("a preset has no ID or the ID of another")
		case name == "" || names[name]:
			return invalidBackup("a preset has no name or the name of another")
		case !slices.Contains([]string{"", ActionProxy, ActionDirect, ActionBlock}, record.Default):
			return invalidBackup("the preset %q sends the rest of the traffic nowhere known", record.Name)
		}
		if _, ok := e.builtin(record.ID); ok {
			return invalidBackup("the preset %q has the ID of a built-in one", record.Name)
		}
		ids[record.ID], names[name] = true, true
		if _, err := checkRules(record.Advanced, record.Endpoints, record.Rules); err != nil {
			return invalidBackup("the preset %q: %v", record.Name, err)
		}
		if _, err := e.compose(record); err != nil {
			return invalidBackup("the preset %q: %v", record.Name, err)
		}
	}
	return nil
}

func backupInfo(file backupFile) BackupInfo {
	info := BackupInfo{Created: file.Created, Version: file.Version, Parts: nonNil(file.Parts), Groups: len(file.Groups), Presets: len(file.Presets), Codes: len(file.OTP)}
	for _, list := range file.Servers {
		info.Servers += len(list)
	}
	return info
}

// InspectBackup tells what a backup holds, after the same checks as a restore.
func (e *Engine) InspectBackup(p BackupText) (BackupInfo, error) {
	file, err := e.readBackup(p.Text)
	if err != nil {
		return BackupInfo{}, err
	}
	return backupInfo(file), nil
}

// RestoreBackup puts the chosen parts of a backup in place of the current ones, each whole, and leaves the rest. The
// connection stops first: its server and preset may go. The chosen server and preset that are gone afterwards give
// way to none and to the standard preset; the rights at sign-in stay as they are, being of this computer.
func (e *Engine) RestoreBackup(p BackupRestore) (BackupInfo, error) {
	file, err := e.readBackup(p.Text)
	if err != nil {
		return BackupInfo{}, err
	}
	parts := p.Parts
	if len(parts) == 0 {
		parts = file.Parts
	}
	for _, part := range parts {
		if !slices.Contains(file.Parts, part) {
			return BackupInfo{}, invalidBackup("the backup has no part %q", part)
		}
	}
	has := func(part string) bool { return slices.Contains(parts, part) }
	if err = e.stopForRestore(); err != nil {
		return BackupInfo{}, err
	}

	e.writeMu.Lock()
	err = e.db.Update(func(tx *bbolt.Tx) error {
		if has(BackupProfiles) {
			for _, bucket := range [][]byte{bucketGroups, bucketServers} {
				if err := clearBucket(tx, bucket); err != nil {
					return err
				}
			}
			for i := range file.Groups {
				servers := nonNil(file.Servers[file.Groups[i].ID])
				if err := storeGroup(tx, &file.Groups[i], servers); err != nil {
					return err
				}
			}
		}
		if has(BackupRouting) {
			if err := clearBucket(tx, bucketPresets); err != nil {
				return err
			}
			for _, record := range file.Presets {
				if err := putJSON(tx, bucketPresets, []byte(record.ID), record); err != nil {
					return err
				}
			}
		}
		if has(BackupOTP) {
			return putJSON(tx, bucketOTP, keyOTP, nonNil(file.OTP))
		}
		return nil
	})
	e.writeMu.Unlock()
	if err != nil {
		return BackupInfo{}, err
	}

	settings, err := e.Settings()
	if err != nil {
		return BackupInfo{}, err
	}
	if has(BackupSettings) {
		restored := *file.Settings
		restored.AdminAutostart = settings.AdminAutostart
		settings = restored
	}
	if _, found, err := e.preset(settings.Preset); err != nil || !found {
		settings.Preset = standardPreset
	}
	if settings.Server != "" {
		if _, found, err := e.serverProfile(settings.Server); err != nil || !found {
			settings.Server = ""
		}
	}
	if _, err = e.SetSettings(settings); err != nil {
		return BackupInfo{}, fmt.Errorf("the settings after the restore: %w", err)
	}
	if groups, err := e.Groups(); err == nil {
		e.notify(EventGroupsChanged, groups)
	}
	e.notify(EventServersChanged, ServersChanged{})
	e.announcePresets()
	e.announceOTP()
	log.Printf("backup: restored %s from a backup of %s", strings.Join(parts, ", "), time.UnixMilli(file.Created).Format(time.DateTime))
	return backupInfo(file), nil
}

func clearBucket(tx *bbolt.Tx, name []byte) error {
	if err := tx.DeleteBucket(name); err != nil && !errors.Is(err, bbolt.ErrBucketNotFound) {
		return err
	}
	_, err := tx.CreateBucket(name)
	return err
}

// stopForRestore stops the connection and waits for it to be down.
func (e *Engine) stopForRestore() error {
	if e.ConnectionState().State == ConnectionIdle {
		return nil
	}
	if _, err := e.StopConnection(); err != nil {
		return err
	}
	deadline := time.Now().Add(stopWait)
	for e.ConnectionState().State != ConnectionIdle {
		if time.Now().After(deadline) {
			return jsonrpc.Failed(ReasonStopTimeout, "the connection did not stop for the restore")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}
