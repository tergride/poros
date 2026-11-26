package engine

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/sagernet/bbolt"

	"PorosCore/config"
	"PorosCore/internal/jsonrpc"
	"PorosCore/profile"
)

var (
	// bucketEndpoints holds the endpoints as one list under keyEndpoints, in the user's order: the order in which their
	// rules are checked.
	bucketEndpoints = []byte("endpoints")
	keyEndpoints    = []byte("entries")
)

// endpointRecord is an endpoint as stored.
type endpointRecord struct {
	ID      string          `json:"id"`
	On      bool            `json:"on"`
	Profile profile.Profile `json:"profile"`
	Rules   []PresetRule    `json:"rules"`
	Need    []string        `json:"need,omitempty"`
}

func (r endpointRecord) view() Endpoint {
	return Endpoint{ID: r.ID, Name: r.Profile.Name, Type: r.Profile.Type, On: r.On, Rules: len(r.Rules), Need: nonNil(r.Need)}
}

func (r endpointRecord) entry() EndpointEntry {
	return EndpointEntry{ID: r.ID, On: r.On, Profile: r.Profile, Rules: nonNil(r.Rules), Need: nonNil(r.Need)}
}

func loadEndpoints(tx *bbolt.Tx) ([]endpointRecord, error) {
	var records []endpointRecord
	if _, err := getJSON(tx, bucketEndpoints, keyEndpoints, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (e *Engine) endpointRecords() (records []endpointRecord, err error) {
	err = e.db.View(func(tx *bbolt.Tx) error {
		records, err = loadEndpoints(tx)
		return err
	})
	return records, err
}

// Endpoints lists the endpoints in the user's order.
func (e *Engine) Endpoints() ([]Endpoint, error) {
	records, err := e.endpointRecords()
	if err != nil {
		return nil, err
	}
	views := make([]Endpoint, 0, len(records))
	for _, record := range records {
		views = append(views, record.view())
	}
	return views, nil
}

func (e *Engine) announceEndpoints() {
	if endpoints, err := e.Endpoints(); err == nil {
		e.notify(EventEndpointsChanged, endpoints)
	}
}

func endpointNotFound(id string) error {
	return jsonrpc.Failed(ReasonNotFound, "there is no endpoint %q", id)
}

func (e *Engine) EndpointEntry(id string) (EndpointEntry, error) {
	records, err := e.endpointRecords()
	if err != nil {
		return EndpointEntry{}, err
	}
	i := slices.IndexFunc(records, func(r endpointRecord) bool { return r.ID == id })
	if i < 0 {
		return EndpointEntry{}, endpointNotFound(id)
	}
	return records[i].entry(), nil
}

// signInOrder is the order Need names the data for signing in in.
var signInOrder = []string{SignInUsername, SignInPassword, SignInCA, SignInCert, SignInKey, SignInKeyPassword, SignInWrapKey, SignInOTP}

// signInFields gives the data for signing in of a profile by the names Need takes, as pointers into a copy of its part
// that the profile now holds.
func signInFields(p *profile.Profile) map[string]*string {
	switch {
	case (p.Type == profile.TypeOpenVPN || p.Type == profile.TypeOpenVPNBY) && p.OpenVPN != nil:
		o := *p.OpenVPN
		p.OpenVPN = &o
		fields := map[string]*string{SignInUsername: &o.Username, SignInPassword: &o.Password, SignInCA: &o.CA, SignInCert: &o.Cert,
			SignInKey: &o.Key, SignInOTP: &o.OTP}
		if p.Type == profile.TypeOpenVPNBY {
			fields[SignInKeyPassword] = &o.KeyPassword
		}
		if o.ControlWrap.Type != "" {
			fields[SignInWrapKey] = &o.ControlWrap.Key
		}
		return fields
	case p.Type == profile.TypeOpenConnect && p.OpenConnect != nil:
		o := *p.OpenConnect
		p.OpenConnect = &o
		return map[string]*string{SignInUsername: &o.Username, SignInPassword: &o.Password, SignInCert: &o.Cert, SignInKey: &o.Key,
			SignInKeyPassword: &o.KeyPassword, SignInOTP: &o.OTP}
	}
	return map[string]*string{}
}

// stillNeeded is what of need the profile lacks yet, in signInOrder; a name it has no such data for is dropped. A code
// of OTP is needed only while {otp} may sign in: the name or the password has it or is still missing. The password of
// the key is needed only while the key is protected by one, or is not there to tell.
func stillNeeded(need []string, p profile.Profile) []string {
	fields := signInFields(&p)
	missing := func(name string) bool {
		field, ok := fields[name]
		return ok && *field == "" && slices.Contains(need, name)
	}
	var left []string
	for _, name := range signInOrder {
		switch {
		case !missing(name):
		case name == SignInOTP && !usesOTP(fields) && !missing(SignInUsername) && !missing(SignInPassword):
		case name == SignInKeyPassword && *fields[SignInKey] != "" && !strings.Contains(*fields[SignInKey], "ENCRYPTED"):
		default:
			left = append(left, name)
		}
	}
	return left
}

// usesOTP tells a profile that signs in with {otp}.
func usesOTP(fields map[string]*string) bool {
	for _, name := range []string{SignInUsername, SignInPassword} {
		if field, ok := fields[name]; ok && strings.Contains(*field, otpPlaceholder) {
			return true
		}
	}
	return false
}

// checkEndpoint reads an endpoint as it is kept: an OpenVPN, OpenVPN|BY or OpenConnect profile with a name, rules of
// an endpoint, and what it still needs to sign in. It cannot be on while it needs any.
func checkEndpoint(entry EndpointEntry) (endpointRecord, error) {
	p := entry.Profile
	p.Name = strings.TrimSpace(p.Name)
	if !profile.IsEndpointType(p.Type) {
		return endpointRecord{}, jsonrpc.Failed(ReasonInvalidValue, "an endpoint is an OpenVPN, OpenVPN|BY or OpenConnect connection, not %q", p.Type)
	}
	if p.Name == "" {
		return endpointRecord{}, jsonrpc.Failed(ReasonInvalidValue, "the endpoint needs a name")
	}
	need := stillNeeded(entry.Need, p)
	if err := waiting(p, need).Validate(); err != nil {
		return endpointRecord{}, jsonrpc.Failed(ReasonInvalidValue, "%s: %v", p.Name, err)
	}
	rules, err := checkRules(endpointRules, entry.Rules)
	if err != nil {
		return endpointRecord{}, err
	}
	record := endpointRecord{ID: entry.ID, On: entry.On, Profile: p, Rules: rules, Need: need}
	if record.On && len(record.Need) > 0 {
		return endpointRecord{}, jsonrpc.Failed(ReasonSignInMissing, "%s lacks data for signing in: %s", p.Name, strings.Join(record.Need, ", "))
	}
	return record, nil
}

// waiting is the profile as it is checked while it needs data of an OpenVPN file: what it waits for is checked once it
// is in, so the CA and the client's pair stand in as empty PEM blocks of their kind, and the protection of the control
// channel waits whole for its key.
func waiting(p profile.Profile, need []string) profile.Profile {
	if p.OpenVPN == nil {
		return p
	}
	o := *p.OpenVPN
	block := func(kind string) string { return "-----BEGIN " + kind + "-----\n-----END " + kind + "-----" }
	for _, name := range need {
		switch name {
		case SignInCA:
			o.CA = block("CERTIFICATE")
		case SignInCert:
			o.Cert = block("CERTIFICATE")
		case SignInKey:
			o.Key = block("PRIVATE KEY")
		case SignInWrapKey:
			o.ControlWrap = profile.OpenVPNControlWrap{}
		}
	}
	p.OpenVPN = &o
	return p
}

func nameTakenBy(records []endpointRecord, name, id string) bool {
	return slices.ContainsFunc(records, func(r endpointRecord) bool { return r.ID != id && strings.EqualFold(r.Profile.Name, name) })
}

// SaveEndpoint keeps an endpoint: a new one at the end of the list, a known one in its place.
func (e *Engine) SaveEndpoint(entry EndpointEntry) (Endpoint, error) {
	record, err := checkEndpoint(entry)
	if err != nil {
		return Endpoint{}, err
	}
	e.writeMu.Lock()
	err = e.db.Update(func(tx *bbolt.Tx) error {
		records, err := loadEndpoints(tx)
		if err != nil {
			return err
		}
		if nameTakenBy(records, record.Profile.Name, record.ID) {
			return jsonrpc.Failed(ReasonEndpointNameTaken, "there is an endpoint named %q already", record.Profile.Name)
		}
		if record.ID == "" {
			record.ID = newID()
			records = append(records, record)
		} else if i := slices.IndexFunc(records, func(r endpointRecord) bool { return r.ID == record.ID }); i >= 0 {
			records[i] = record
		} else {
			return endpointNotFound(record.ID)
		}
		return putJSON(tx, bucketEndpoints, keyEndpoints, records)
	})
	e.writeMu.Unlock()
	if err != nil {
		return Endpoint{}, err
	}
	e.announceEndpoints()
	return record.view(), nil
}

// RemoveEndpoint forgets an endpoint.
func (e *Engine) RemoveEndpoint(id string) error {
	return e.changeEndpoints(id, func(records []endpointRecord, i int) []endpointRecord { return slices.Delete(records, i, i+1) })
}

// MoveEndpoint moves an endpoint by one place; past either end of the list it stays.
func (e *Engine) MoveEndpoint(p EndpointMove) error {
	return e.changeEndpoints(p.ID, func(records []endpointRecord, i int) []endpointRecord {
		if to := i + p.Offset; to >= 0 && to < len(records) {
			records[i], records[to] = records[to], records[i]
		}
		return records
	})
}

func (e *Engine) changeEndpoints(id string, change func([]endpointRecord, int) []endpointRecord) error {
	e.writeMu.Lock()
	err := e.db.Update(func(tx *bbolt.Tx) error {
		records, err := loadEndpoints(tx)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(records, func(r endpointRecord) bool { return r.ID == id })
		if i < 0 {
			return endpointNotFound(id)
		}
		return putJSON(tx, bucketEndpoints, keyEndpoints, change(records, i))
	})
	e.writeMu.Unlock()
	if err == nil {
		e.announceEndpoints()
	}
	return err
}

const (
	endpointFileKind = "poros-endpoint"
	// maxEndpointFile bounds a file to import: an endpoint with its certificates and rules is far less.
	maxEndpointFile = 1 << 20
)

// endpointFile is an endpoint as a file: its connection, its rules, and the data for signing in it was written
// without. The ID of a code of OTP is of this computer and never goes in.
type endpointFile struct {
	Kind    string          `json:"kind"`
	Version int             `json:"v"`
	Profile profile.Profile `json:"profile"`
	Rules   []linkRule      `json:"rules,omitempty"`
	Need    []string        `json:"need,omitempty"`
}

// ExportEndpoint writes an endpoint as a file. Without the data for signing in, the file names what it left out, so
// that an import asks for exactly that.
func (e *Engine) ExportEndpoint(p EndpointExport) (EndpointFile, error) {
	entry, err := e.EndpointEntry(p.ID)
	if err != nil {
		return EndpointFile{}, err
	}
	prof := entry.Profile
	fields := signInFields(&prof)
	need := slices.Clone(entry.Need)
	if otp := fields[SignInOTP]; otp != nil && *otp != "" {
		*otp = ""
		need = append(need, SignInOTP)
	}
	if !p.SignIn {
		for _, name := range signInOrder {
			if field, ok := fields[name]; ok && personal(prof, name) && *field != "" {
				*field = ""
				need = append(need, name)
			}
		}
	}
	data, err := json.MarshalIndent(endpointFile{
		Kind: endpointFileKind, Version: 1, Profile: prof, Rules: linkRules(entry.Rules), Need: stillNeeded(need, prof),
	}, "", "  ")
	if err != nil {
		return EndpointFile{}, err
	}
	return EndpointFile{Name: fileName(prof.Name) + ".json", Text: string(data) + "\n"}, nil
}

// personal tells the data for signing in that is the user's own and stays out of a file without it. The CA and the
// keys of tls-auth and tls-crypt are the server's, shared by all its clients; the key of tls-crypt-v2 is the client's
// own. The code of OTP is this computer's, and stays out of any file.
func personal(p profile.Profile, name string) bool {
	switch name {
	case SignInOTP, SignInCA:
		return false
	case SignInWrapKey:
		return p.OpenVPN != nil && p.OpenVPN.ControlWrap.Type == "tls_crypt_v2"
	}
	return true
}

// ImportEndpoint adds an endpoint, off, from the file of one or from an .ovpn file. It needs the data for signing in
// the file came without; a name that is taken gets a number: "Office 2".
func (e *Engine) ImportEndpoint(p EndpointText) (Endpoint, error) {
	text := strings.TrimSpace(p.Text)
	if len(text) > maxEndpointFile {
		return Endpoint{}, jsonrpc.Failed(ReasonInvalidLink, "the file is larger than %d KB", maxEndpointFile>>10)
	}
	var entry EndpointEntry
	var err error
	if strings.HasPrefix(text, "{") {
		entry, err = readEndpointFile(text)
	} else {
		entry, err = readOpenVPNFile(text)
	}
	if err != nil {
		return Endpoint{}, err
	}
	// A code of OTP is chosen on this computer: {otp} waits for it.
	fields := signInFields(&entry.Profile)
	if otp := fields[SignInOTP]; otp != nil {
		*otp = ""
		if usesOTP(fields) {
			entry.Need = append(entry.Need, SignInOTP)
		}
	}
	records, err := e.endpointRecords()
	if err != nil {
		return Endpoint{}, err
	}
	base := cmp.Or(strings.TrimSpace(entry.Profile.Name), entry.Profile.Server)
	entry.Profile.Name = base
	for n := 2; nameTakenBy(records, entry.Profile.Name, ""); n++ {
		entry.Profile.Name = fmt.Sprintf("%s %d", base, n)
	}
	entry.ID, entry.On = "", false
	return e.SaveEndpoint(entry)
}

// readEndpointFile reads the file of an endpoint. Unknown keys are refused: a rule that lost a condition would match
// more.
func readEndpointFile(text string) (EndpointEntry, error) {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	var file endpointFile
	if err := decoder.Decode(&file); err != nil || file.Kind != endpointFileKind || file.Version != 1 {
		return EndpointEntry{}, jsonrpc.Failed(ReasonInvalidLink, "it holds no Poros endpoint")
	}
	rules := make([]PresetRule, len(file.Rules))
	for i, r := range file.Rules {
		rules[i] = PresetRule(r)
	}
	return EndpointEntry{Profile: file.Profile, Rules: rules, Need: file.Need}, nil
}

// readOpenVPNFile reads an .ovpn file as an endpoint without rules. It needs what it reads from files of their own, a
// name and a password it asks for and has not got, and the password of a key that has one.
func readOpenVPNFile(text string) (EndpointEntry, error) {
	p, files, err := profile.ReadOpenVPNConfig([]byte(text))
	if err != nil {
		return EndpointEntry{}, jsonrpc.Failed(ReasonInvalidLink, "it holds no Poros endpoint and no OpenVPN config: %v", err)
	}
	entry := EndpointEntry{Profile: p, Rules: []PresetRule{}}
	signIn := p.OpenVPN.Username == "" && (files.Credentials || asksSignIn(text))
	for _, need := range []struct {
		name string
		on   bool
	}{
		{SignInUsername, signIn}, {SignInPassword, signIn}, {SignInCA, files.CA}, {SignInCert, files.Cert}, {SignInKey, files.Key},
		{SignInKeyPassword, true}, {SignInWrapKey, files.WrapKey},
	} {
		if need.on {
			entry.Need = append(entry.Need, need.name)
		}
	}
	return entry, nil
}

// asksSignIn tells an .ovpn file with auth-user-pass: the server asks for a name and a password.
func asksSignIn(text string) bool {
	for line := range strings.Lines(text) {
		if fields := strings.Fields(line); len(fields) > 0 && strings.EqualFold(fields[0], "auth-user-pass") {
			return true
		}
	}
	return false
}

// endpointsOn gives the endpoints a connection runs with: those that are on, in the user's order.
func (e *Engine) endpointsOn() ([]endpointRecord, error) {
	records, err := e.endpointRecords()
	return slices.DeleteFunc(records, func(r endpointRecord) bool { return !r.On || len(r.Need) > 0 }), err
}

// withEndpoints adds the endpoints to the preset of a connection: the rules of each go to its outbound, right after
// the DNS queries and before the preset's own rules, in the order of the endpoints. The config builder puts the
// networks their servers announce first of all.
func withEndpoints(preset config.Preset, endpoints []endpointRecord) config.Preset {
	if len(endpoints) == 0 {
		return preset
	}
	var rules []config.Rule
	preset.Endpoints = make([]string, len(endpoints))
	for i, endpoint := range endpoints {
		preset.Endpoints[i] = endpoint.ID
		for _, rule := range endpoint.Rules {
			if !rule.Off {
				rules = append(rules, configRule(rule, config.EndpointTag(i)))
			}
		}
	}
	n := head(preset)
	preset.Rules = slices.Concat(preset.Rules[:n], rules, preset.Rules[n:])
	return preset
}

// connectionPreset is the preset a connection or the refusal of the kill switch runs with: the chosen one with the
// endpoints that are on.
func (e *Engine) connectionPreset(id string) (config.Preset, []endpointRecord, bool, error) {
	preset, found, err := e.preset(id)
	if err != nil || !found {
		return preset, nil, found, err
	}
	endpoints, err := e.endpointsOn()
	if err != nil {
		return config.Preset{}, nil, false, err
	}
	return withEndpoints(preset, endpoints), endpoints, true, nil
}
