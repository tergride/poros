package engine

import (
	"slices"
	"strings"
	"time"

	"github.com/sagernet/bbolt"

	"PorosCore/internal/jsonrpc"
	"PorosCore/internal/otp"
	"PorosCore/profile"
)

var (
	// bucketOTP holds the codes of «Tools → OTP» as one list under keyOTP, in the user's order.
	bucketOTP = []byte("otp")
	keyOTP    = []byte("entries")
)

// otpPlaceholder in the user name or password of an OpenVPN or OpenConnect profile stands for its code of OTP.
const otpPlaceholder = "{otp}"

func loadOTP(tx *bbolt.Tx) ([]OTPEntry, error) {
	var entries []OTPEntry
	if _, err := getJSON(tx, bucketOTP, keyOTP, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (e OTPEntry) entry() otp.Entry {
	return otp.Entry{Name: e.Name, Account: e.Account, Secret: e.Secret, Kind: e.Kind, Algorithm: e.Algorithm,
		Digits: e.Digits, Period: e.Period, Counter: e.Counter}
}

func otpEntryOf(id string, e otp.Entry) OTPEntry {
	return OTPEntry{ID: id, Name: e.Name, Account: e.Account, Secret: e.Secret, Kind: e.Kind, Algorithm: e.Algorithm,
		Digits: e.Digits, Period: e.Period, Counter: e.Counter}
}

func otpProblemOf(p otp.Problem) OTPProblem {
	return OTPProblem{Line: p.Line, Name: p.Name, Reason: p.Reason, Value: p.Value}
}

// code is what the list shows of an entry: a TOTP code with its end, nothing for HOTP.
func (e OTPEntry) code(now time.Time) OTPCode {
	c := OTPCode{ID: e.ID, Name: e.Name, Account: e.Account, Kind: e.Kind, Digits: e.Digits, Period: e.Period}
	if e.Kind == otp.KindTOTP {
		code, until := e.entry().TOTP(now)
		c.Code, c.Until = code, until.UnixMilli()
	}
	return c
}

func (e *Engine) OTPCodes() ([]OTPCode, error) {
	var entries []OTPEntry
	err := e.db.View(func(tx *bbolt.Tx) (err error) {
		entries, err = loadOTP(tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	now := e.now()
	codes := make([]OTPCode, 0, len(entries))
	for _, entry := range entries {
		codes = append(codes, entry.code(now))
	}
	return codes, nil
}

func (e *Engine) announceOTP() {
	if codes, err := e.OTPCodes(); err == nil {
		e.notify(EventOTPChanged, codes)
	}
}

func (e *Engine) OTPEntry(id string) (OTPEntry, error) {
	var entry OTPEntry
	var found bool
	err := e.db.View(func(tx *bbolt.Tx) error {
		entries, err := loadOTP(tx)
		if i := slices.IndexFunc(entries, func(x OTPEntry) bool { return x.ID == id }); i >= 0 {
			entry, found = entries[i], true
		}
		return err
	})
	if err == nil && !found {
		err = jsonrpc.Failed(ReasonNotFound, "there is no code of OTP %q", id)
	}
	return entry, err
}

// ParseOTP reads text with codes; nothing is added.
func (e *Engine) ParseOTP(p OTPText) (OTPParsed, error) {
	entries, problems := otp.Parse(p.Text)
	parsed := OTPParsed{Entries: make([]OTPEntry, 0, len(entries)), Problems: make([]OTPProblem, 0, len(problems))}
	for _, entry := range entries {
		parsed.Entries = append(parsed.Entries, otpEntryOf("", entry))
	}
	for _, problem := range problems {
		parsed.Problems = append(parsed.Problems, otpProblemOf(problem))
	}
	return parsed, nil
}

// sameOTP tells an entry Poros already has: the same secret for the same service and account gives the same codes.
func sameOTP(a, b OTPEntry) bool {
	return a.Secret == b.Secret && a.Kind == b.Kind && a.Name == b.Name && a.Account == b.Account
}

// ImportOTP adds the codes text holds, after those there are; one that is there already is skipped.
func (e *Engine) ImportOTP(p OTPText) (OTPImported, error) {
	parsed, _ := e.ParseOTP(p)
	result := OTPImported{Problems: parsed.Problems}
	e.writeMu.Lock()
	err := e.db.Update(func(tx *bbolt.Tx) error {
		entries, err := loadOTP(tx)
		if err != nil {
			return err
		}
		for _, entry := range parsed.Entries {
			if slices.ContainsFunc(entries, func(x OTPEntry) bool { return sameOTP(x, entry) }) {
				result.Duplicates++
				continue
			}
			entry.ID = newID()
			entries = append(entries, entry)
			result.Added++
		}
		if result.Added == 0 {
			return nil
		}
		return putJSON(tx, bucketOTP, keyOTP, entries)
	})
	e.writeMu.Unlock()
	if err != nil {
		return OTPImported{}, err
	}
	if result.Added > 0 {
		e.announceOTP()
	}
	return result, nil
}

func invalidOTP(problem otp.Problem) error {
	return &jsonrpc.Error{Code: jsonrpc.CodeFailed, Message: "the code of OTP cannot be saved: " + problem.Error(),
		Data: OTPFailure{Reason: ReasonInvalidOTP, Problem: otpProblemOf(problem)}}
}

// SaveOTP keeps an entry: a new one at the end of the list, a known one in its place. The secret may come as an
// authenticator app prints it; it is kept canonical.
func (e *Engine) SaveOTP(entry OTPEntry) (OTPEntry, error) {
	entry.Name, entry.Account = strings.TrimSpace(entry.Name), strings.TrimSpace(entry.Account)
	if secret, ok := otp.Normalize(entry.Secret); ok {
		entry.Secret = secret
	}
	if entry.Kind == otp.KindHOTP {
		entry.Period = 0
	} else {
		entry.Counter = 0
	}
	if problem := entry.entry().Check(); problem != nil {
		return OTPEntry{}, invalidOTP(*problem)
	}
	e.writeMu.Lock()
	err := e.db.Update(func(tx *bbolt.Tx) error {
		entries, err := loadOTP(tx)
		if err != nil {
			return err
		}
		if entry.ID == "" {
			entry.ID = newID()
			entries = append(entries, entry)
		} else if i := slices.IndexFunc(entries, func(x OTPEntry) bool { return x.ID == entry.ID }); i >= 0 {
			entries[i] = entry
		} else {
			return jsonrpc.Failed(ReasonNotFound, "there is no code of OTP %q", entry.ID)
		}
		return putJSON(tx, bucketOTP, keyOTP, entries)
	})
	e.writeMu.Unlock()
	if err != nil {
		return OTPEntry{}, err
	}
	e.announceOTP()
	return entry, nil
}

// RemoveOTP forgets an entry. Profiles that chose it keep its ID: their start then says the code is gone.
func (e *Engine) RemoveOTP(id string) error {
	return e.changeOTP(id, func(entries []OTPEntry, i int) []OTPEntry { return slices.Delete(entries, i, i+1) })
}

// MoveOTP moves an entry by one place; past either end of the list it stays.
func (e *Engine) MoveOTP(p OTPMove) error {
	return e.changeOTP(p.ID, func(entries []OTPEntry, i int) []OTPEntry {
		to := i + p.Offset
		if to >= 0 && to < len(entries) {
			entries[i], entries[to] = entries[to], entries[i]
		}
		return entries
	})
}

func (e *Engine) changeOTP(id string, change func([]OTPEntry, int) []OTPEntry) error {
	e.writeMu.Lock()
	err := e.db.Update(func(tx *bbolt.Tx) error {
		entries, err := loadOTP(tx)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(entries, func(x OTPEntry) bool { return x.ID == id })
		if i < 0 {
			return jsonrpc.Failed(ReasonNotFound, "there is no code of OTP %q", id)
		}
		return putJSON(tx, bucketOTP, keyOTP, change(entries, i))
	})
	e.writeMu.Unlock()
	if err == nil {
		e.announceOTP()
	}
	return err
}

// OTPCodeNow gives the code to use now: TOTP's of the moment, or the next HOTP code, which moves its counter.
func (e *Engine) OTPCodeNow(id string) (OTPCode, error) {
	var code OTPCode
	var spent bool
	e.writeMu.Lock()
	err := e.db.Update(func(tx *bbolt.Tx) error {
		entries, err := loadOTP(tx)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(entries, func(x OTPEntry) bool { return x.ID == id })
		if i < 0 {
			return jsonrpc.Failed(ReasonNotFound, "there is no code of OTP %q", id)
		}
		code = entries[i].code(e.now())
		if entries[i].Kind != otp.KindHOTP {
			return nil
		}
		code.Code = entries[i].entry().Code(entries[i].Counter)
		entries[i].Counter++
		spent = true
		return putJSON(tx, bucketOTP, keyOTP, entries)
	})
	e.writeMu.Unlock()
	if spent {
		e.announceOTP()
	}
	return code, err
}

func (e *Engine) OTPLink(id string) (OTPLink, error) {
	entry, err := e.OTPEntry(id)
	if err != nil {
		return OTPLink{}, err
	}
	return OTPLink{Link: entry.entry().Link()}, nil
}

// otpCodes gives the codes of {otp} for the sign-ins of one start or test. A test takes TOTP codes alone: each HOTP
// code would move the counter of the server.
type otpCodes struct {
	entries []OTPEntry
	now     time.Time
	test    bool
	// spent tells that a HOTP counter moved, and entries must be kept.
	spent bool
}

func (c *otpCodes) code(p profile.Profile, id string) (string, error) {
	name := serverRecord{Profile: p}.view("").Name
	i := slices.IndexFunc(c.entries, func(x OTPEntry) bool { return x.ID == id })
	switch {
	case id == "" || i < 0:
		return "", failure(ReasonOTPMissing, "%s signs in with {otp}, but no code of OTP is chosen for it, or the chosen one is gone", name)
	case c.entries[i].Kind != otp.KindHOTP:
		code, _ := c.entries[i].entry().TOTP(c.now)
		return code, nil
	case c.test:
		return "", failure(ReasonStartFailed, "%s signs in with a HOTP code, which a test does not spend", name)
	}
	code := c.entries[i].entry().Code(c.entries[i].Counter)
	c.entries[i].Counter++
	c.spent = true
	return code, nil
}

// fill puts the code of a profile's OTP into {otp} of its user name and password, in a copy of its part.
func (c *otpCodes) fill(p profile.Profile) (profile.Profile, error) {
	var user, password *string
	var id string
	switch {
	case p.Type == profile.TypeOpenVPN && p.OpenVPN != nil:
		o := *p.OpenVPN
		p.OpenVPN, user, password, id = &o, &o.Username, &o.Password, o.OTP
	case p.Type == profile.TypeOpenConnect && p.OpenConnect != nil:
		o := *p.OpenConnect
		p.OpenConnect, user, password, id = &o, &o.Username, &o.Password, o.OTP
	default:
		return p, nil
	}
	if !strings.Contains(*user, otpPlaceholder) && !strings.Contains(*password, otpPlaceholder) {
		return p, nil
	}
	code, err := c.code(p, id)
	if err != nil {
		return p, err
	}
	*user = strings.ReplaceAll(*user, otpPlaceholder, code)
	*password = strings.ReplaceAll(*password, otpPlaceholder, code)
	return p, nil
}

// fillServer puts the codes into a server and the servers it connects through, in copies.
func (c *otpCodes) fillServer(p profile.Profile, links []profile.Profile) (profile.Profile, []profile.Profile, error) {
	p, err := c.fill(p)
	if err != nil {
		return p, nil, err
	}
	links = slices.Clone(links)
	for i := range links {
		if links[i], err = c.fill(links[i]); err != nil {
			return p, nil, err
		}
	}
	return p, links, nil
}

// fillOTP puts the codes into the profiles of a start; the HOTP counters it spent are kept.
func (e *Engine) fillOTP(profiles ...[]profile.Profile) error {
	e.writeMu.Lock()
	var spent bool
	err := e.db.Update(func(tx *bbolt.Tx) error {
		entries, err := loadOTP(tx)
		if err != nil {
			return err
		}
		codes := &otpCodes{entries: entries, now: e.now()}
		for _, list := range profiles {
			for i := range list {
				if list[i], err = codes.fill(list[i]); err != nil {
					return err
				}
			}
		}
		if spent = codes.spent; !spent {
			return nil
		}
		return putJSON(tx, bucketOTP, keyOTP, codes.entries)
	})
	e.writeMu.Unlock()
	if err == nil && spent {
		e.announceOTP()
	}
	return err
}
