package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"net/http"
	"reflect"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"PorosCore/internal/jsonrpc"
	"PorosCore/profile"
)

// callLimit bounds one call: the shell waits 30 seconds for an answer, and no method should come near that.
const callLimit = 10 * time.Second

// outcome is how a call ended.
type outcome struct {
	result   any
	err      error
	panicked any
	stack    []byte
	hung     bool
}

// callHandler calls a handler as the JSON-RPC server does, with the params it would read off the pipe, but catches
// a panic instead of answering with it and gives up after limit.
func callHandler(handler jsonrpc.Handler, raw []byte, limit time.Duration) outcome {
	done := make(chan outcome, 1)
	go func() {
		var out outcome
		defer func() {
			if r := recover(); r != nil {
				out.panicked, out.stack = r, debug.Stack()
			}
			done <- out
		}()
		out.result, out.err = handler(context.Background(), json.RawMessage(raw))
	}()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case out := <-done:
		return out
	case <-timer.C:
		return outcome{hung: true}
	}
}

// Classes of errors: params that did not decode are a bug of the client and carry no reason; a failure carries a
// reason of api.go, or one api.go does not have; a bare error reaches the interface without a reason.
const (
	errParams  = "params"
	errReason  = "reason"
	errUnknown = "unknown"
	errBare    = "bare"
)

func classify(err error, reasons map[string]string) (class, detail string) {
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		return errBare, err.Error()
	}
	switch rpcErr.Code {
	case jsonrpc.CodeInvalidParams:
		return errParams, rpcErr.Message
	case jsonrpc.CodeFailed:
		r := reason(err)
		switch {
		case r == "":
			return errBare, rpcErr.Message
		case reasons[r] == "":
			return errUnknown, r
		}
		return errReason, r
	}
	return errBare, rpcErr.Message
}

func shown(raw []byte) string {
	if len(raw) <= 400 {
		return strconv.Quote(string(raw))
	}
	return fmt.Sprintf("%s... (%d bytes)", strconv.Quote(string(raw[:400])), len(raw))
}

func trojanTo(address, name string) string {
	return "trojan://secret@" + address + "?security=none#" + name
}

func xhttpTo(address, name string) string {
	return "vless://" + testUUID + "@" + address + "?type=xhttp&path=%2Fx&security=none#" + name
}

func ssTo(address, name string) string {
	return "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:secret")) + "@" + address + "#" + name
}

// contractMethod is how the contract test calls a method: with params of the type of params, random or a valid
// value of base changed a little. A method that reaches the network runs on the engine whose servers lead to a
// loopback port that closes at once and whose settings no call changes (onNet). sanitize keeps an input from
// turning on what this engine leaves alone. A method that cannot be made safe on the host is skipped.
type contractMethod struct {
	params   any
	base     func(f *contractFuzz) any
	onNet    bool
	skip     string
	sanitize func(raw []byte) []byte
}

// safeSettings turns off the kill switch and the task of the sign-in in settings that turn them on: the kill switch
// would run a refusal with what else the settings say, the network included, and the task is the system's.
func safeSettings(s *Settings) bool {
	changed := false
	if s.KillSwitch == KillSwitchConnected || s.KillSwitch == KillSwitchAlways {
		s.KillSwitch, changed = KillSwitchOff, true
	}
	if s.AdminAutostart {
		s.AdminAutostart, changed = false, true
	}
	return changed
}

// sanitizeSettings decodes the params as settings.set does and makes them safe when they would decode.
func sanitizeSettings(raw []byte) []byte {
	var s Settings
	if jsonrpc.DecodeParams(raw, &s) != nil || !safeSettings(&s) {
		return raw
	}
	data, _ := json.Marshal(s)
	return data
}

// sanitizeRestore reads the backup as backup.restore does and makes its settings safe when it would read.
func sanitizeRestore(raw []byte) []byte {
	var p BackupRestore
	if jsonrpc.DecodeParams(raw, &p) != nil {
		return raw
	}
	var file backupFile
	if json.Unmarshal([]byte(strings.TrimSpace(p.Text)), &file) != nil || file.Settings == nil || !safeSettings(file.Settings) {
		return raw
	}
	text, _ := json.Marshal(file)
	p.Text = string(text)
	data, _ := json.Marshal(p)
	return data
}

func contractMethods() map[string]contractMethod {
	storeServers := func(f *contractFuzz) []string { return f.pool.storeServers }
	storeGroups := func(f *contractFuzz) []string { return f.pool.storeGroups }
	presets := func(f *contractFuzz) []string { return f.pool.presets }
	codes := func(f *contractFuzz) []string { return f.pool.otp }
	byID := func(list func(f *contractFuzz) []string) func(f *contractFuzz) any {
		return func(f *contractFuzz) any { return ByID{ID: f.pickID(list(f))} }
	}
	serverIDs := func(f *contractFuzz) any { return ServerIDs{IDs: f.someIDs(f.pool.storeServers)} }
	netQuery := func(f *contractFuzz) any {
		var q ServersQuery
		if f.chance(0.5) {
			q.GroupID = f.pickID(f.pool.netGroups)
		}
		if f.chance(0.3) {
			q.IDs = f.someIDs(f.pool.netServers)
		}
		return q
	}
	saveServer := func(f *contractFuzz) any {
		ids := slices.Sorted(maps.Keys(f.pool.profiles))
		if len(ids) == 0 || f.chance(0.15) {
			return SaveServer{GroupID: f.pickID(f.pool.manual), Profile: chainProfile(f.someIDs(f.pool.storeServers)...)}
		}
		id := ids[f.r.IntN(len(ids))]
		req := SaveServer{ID: id, Profile: f.pool.profiles[id]}
		if f.chance(0.3) {
			req.ID, req.GroupID = "", f.pickID(f.pool.manual)
		}
		return req
	}
	presetCopy := func(f *contractFuzz) PresetCopy {
		c, err := f.store.PresetCopy(f.pickID(f.pool.presets))
		if err != nil || c.Builtin {
			return PresetCopy{BasedOn: standardPreset, Name: fmt.Sprintf("Copy %d", f.r.IntN(1000)),
				Rules: []PresetRule{rule(CondSuffix, "example.org", ActionDirect), rule(CondIP, "10.0.0.0/8", ActionBlock)}}
		}
		return c
	}
	presetRules := func(f *contractFuzz) any {
		c := presetCopy(f)
		return PresetRules{Advanced: c.Advanced, Rules: c.Rules, Endpoints: c.Endpoints}
	}
	otpText := func(f *contractFuzz) any {
		return OTPText{Text: "otpauth://totp/GitHub:me?secret=JBSWY3DPEHPK3PXP&issuer=GitHub\n" +
			"otpauth://hotp/Office?secret=KRSXG5BAMFZXG5BA&counter=4\nJBSWY3DPEHPK3PXP"}
	}
	backupText := func(f *contractFuzz) any { return BackupText{Text: f.pool.backup} }
	none := contractMethod{}
	net := contractMethod{onNet: true}
	return map[string]contractMethod{
		MethodAppInfo:      none,
		MethodAppHandOver:  none,
		MethodAppDebugInfo: none,
		MethodSettingsGet:  none,
		MethodSettingsSet: {params: Settings{}, sanitize: sanitizeSettings, base: func(f *contractFuzz) any {
			s, _ := f.store.Settings()
			switch f.r.IntN(5) {
			case 0:
				s.Server = f.pickID(f.pool.storeServers)
			case 1:
				s.Preset = f.pickID(f.pool.presets)
			case 2:
				s.LogLines = 100 + f.r.IntN(1000)
			case 3:
				s.UIMode = "advanced"
			}
			return s
		}},
		MethodDNSCheck: {params: DNSObject{}, base: func(f *contractFuzz) any {
			return DNSObject{Object: `{"servers":[{"type":"udp","tag":"u","server":"1.1.1.1"}],"final":"u"}`}
		}},
		MethodWARPRegister: {params: WARPRegister{}, onNet: true, base: func(f *contractFuzz) any {
			return WARPRegister{Mode: []string{WARPModeWireGuard, WARPModeMASQUE}[f.r.IntN(2)]}
		}},
		MethodGroupsList: none,
		MethodGroupsAdd: {params: AddGroup{}, base: func(f *contractFuzz) any {
			return AddGroup{Name: fmt.Sprintf("Group %d", f.r.IntN(1000))}
		}},
		MethodGroupsRename: {params: RenameGroup{}, base: func(f *contractFuzz) any {
			return RenameGroup{ID: f.pickID(f.pool.storeGroups), Name: fmt.Sprintf("Renamed %d", f.r.IntN(1000))}
		}},
		MethodGroupsRemove: {params: ByID{}, base: byID(storeGroups)},
		MethodGroupsConfigure: {params: ConfigureGroup{}, base: func(f *contractFuzz) any {
			c := ConfigureGroup{ID: f.pickID(f.pool.storeGroups), Settings: GroupSettings{
				Update: []string{"", GroupUpdateProvider, GroupUpdateHours, GroupUpdateNever}[f.r.IntN(4)], UpdateHours: f.r.IntN(30),
			}}
			if f.chance(0.3) {
				c.URL = f.provider + "?n=" + strconv.Itoa(f.r.IntN(100))
			}
			if f.chance(0.3) {
				c.Settings.FrontProxy = f.pickID(f.pool.storeServers)
			}
			if f.chance(0.2) {
				c.Settings.LandingProxy = f.pickID(f.pool.storeServers)
			}
			if f.chance(0.2) {
				c.Settings.ServerAddress = []string{"127.0.0.1", "cdn.example.com", "[::1]"}[f.r.IntN(3)]
			}
			return c
		}},
		MethodSubscriptionsAdd: {params: AddSubscription{}, onNet: true, base: func(f *contractFuzz) any {
			return AddSubscription{URL: f.provider + "?n=" + strconv.Itoa(f.r.IntN(1000))}
		}},
		MethodSubscriptionsUpdate: {params: ByID{}, onNet: true, base: func(f *contractFuzz) any { return ByID{ID: f.pickID(f.pool.subs)} }},
		MethodServersList: {params: ServersQuery{}, base: func(f *contractFuzz) any {
			return ServersQuery{GroupID: f.pickID(f.pool.storeGroups)}
		}},
		MethodServersAdd: {params: AddServers{}, base: func(f *contractFuzz) any {
			if f.chance(0.3) {
				return AddServers{NewGroup: "New", Text: f.seedLinks}
			}
			return AddServers{GroupID: f.pickID(f.pool.manual), Text: f.seedLinks}
		}},
		MethodServersGet:    {params: ByID{}, base: byID(storeServers)},
		MethodServersSave:   {params: SaveServer{}, base: saveServer},
		MethodServersRemove: {params: ServerIDs{}, base: serverIDs},
		MethodServersClone:  {params: ServerIDs{}, base: serverIDs},
		MethodServersMove: {params: MoveServers{}, base: func(f *contractFuzz) any {
			return MoveServers{IDs: f.someIDs(f.pool.storeServers), GroupID: f.pickID(f.pool.manual)}
		}},
		MethodServersLinks:     {params: ServerIDs{}, base: serverIDs},
		MethodServersDeepLinks: {params: ServerIDs{}, base: serverIDs},
		MethodServersExport:    {params: ByID{}, base: byID(storeServers)},
		MethodServersForms:     none,
		MethodServersCheck:     {params: SaveServer{}, base: saveServer},
		MethodPresetsList:      none,
		MethodPresetsGet:       {params: ByID{}, base: byID(presets)},
		MethodPresetsSave: {params: PresetCopy{}, base: func(f *contractFuzz) any {
			c := presetCopy(f)
			if f.chance(0.5) {
				c.ID, c.Name = "", fmt.Sprintf("Copy %d", f.r.IntN(1000))
			}
			return c
		}},
		MethodPresetsRemove:     {params: ByID{}, base: func(f *contractFuzz) any { return ByID{ID: f.pickID(f.pool.copies)} }},
		MethodPresetsExport:     {params: ByID{}, base: byID(presets)},
		MethodPresetsImport:     {params: PresetLink{}, base: func(f *contractFuzz) any { return PresetLink{Link: f.pool.presetLink} }},
		MethodPresetsCheck:      {params: PresetRules{}, base: presetRules},
		MethodPresetsWriteLists: {params: PresetRules{}, base: presetRules},
		MethodPresetsReadLists: {params: PresetListsInput{}, base: func(f *contractFuzz) any {
			c := presetCopy(f)
			lists, _ := WritePresetLists(PresetRules{Advanced: c.Advanced, Rules: c.Rules, Endpoints: c.Endpoints})
			return PresetListsInput{Advanced: c.Advanced, Rules: c.Rules, Lists: lists.Lists}
		}},
		MethodLinksParse: {params: OutsideLink{}, base: func(f *contractFuzz) any {
			links := []string{f.pool.deepLink, f.pool.presetLink,
				"poros://addsub/" + base64.RawURLEncoding.EncodeToString([]byte(f.provider+"#Provider")),
				"throne://addsub/" + base64.RawStdEncoding.EncodeToString([]byte(f.provider)),
				"throne://add/" + trojanTo(f.dead, "Throne")}
			return OutsideLink{Link: links[f.r.IntN(len(links))]}
		}},
		MethodServersTest:         {params: ServersQuery{}, onNet: true, base: netQuery},
		MethodServersSpeedTest:    {params: ServersQuery{}, onNet: true, base: netQuery},
		MethodServersIPTest:       {params: ServersQuery{}, onNet: true, base: netQuery},
		MethodServersResolve:      {params: ServersQuery{}, onNet: true, base: netQuery},
		MethodServersClearResults: {params: ServersQuery{}, onNet: true, base: netQuery},
		MethodServersStopTests:    net,
		MethodServersResetTraffic: {params: ServersQuery{}, onNet: true, base: netQuery},
		MethodServersClean: {params: CleanServers{}, base: func(f *contractFuzz) any {
			return CleanServers{GroupID: f.pickID(f.pool.storeGroups), Unavailable: f.chance(0.5), Duplicates: f.chance(0.5), Insecure: f.chance(0.3)}
		}},
		MethodLogsGet:          {params: LogsQuery{}, base: func(f *contractFuzz) any { return LogsQuery{After: int64(f.r.IntN(1 << 20))} }},
		MethodConnectionStart:  net,
		MethodConnectionStop:   net,
		MethodConnectionsList:  net,
		MethodStatsNow:         net,
		MethodStatsTraffic:     {params: TrafficQuery{}, onNet: true, base: func(f *contractFuzz) any { return TrafficQuery{Hours: 1 + f.r.IntN(48)} }},
		MethodConnectionsClose: {params: ByID{}, onNet: true, base: func(f *contractFuzz) any { return ByID{ID: f.pickID(f.pool.conns)} }},
		MethodConnectionState:  net,
		// The engine's settings keep the check off: the services of the check are outside.
		MethodNetworkCheck:    net,
		MethodNetworkState:    net,
		MethodKillSwitchState: net,
		MethodUpdatesState:    none,
		// The engine has no version: it neither looks for nor installs a release.
		MethodUpdatesCheck:   none,
		MethodUpdatesInstall: none,
		MethodOTPList:        none,
		MethodOTPGet:         {params: ByID{}, base: byID(codes)},
		MethodOTPParse:       {params: OTPText{}, base: otpText},
		MethodOTPImport:      {params: OTPText{}, base: otpText},
		MethodOTPSave: {params: OTPEntry{}, base: func(f *contractFuzz) any {
			if entry, err := f.store.OTPEntry(f.pickID(f.pool.otp)); err == nil && f.chance(0.6) {
				return entry
			}
			return totp(fmt.Sprintf("Code %d", f.r.IntN(1000)), "JBSWY3DPEHPK3PXP")
		}},
		MethodOTPRemove: {params: ByID{}, base: byID(codes)},
		MethodOTPMove: {params: OTPMove{}, base: func(f *contractFuzz) any {
			return OTPMove{ID: f.pickID(f.pool.otp), Offset: []int{-1, 1}[f.r.IntN(2)]}
		}},
		MethodOTPCode:       {params: ByID{}, base: byID(codes)},
		MethodOTPLink:       {params: ByID{}, base: byID(codes)},
		MethodQREncode:      {params: QRText{}, base: func(f *contractFuzz) any { return QRText{Text: f.pool.deepLink} }},
		MethodQRScan:        {skip: "it takes a snapshot of the user's screens"},
		MethodBackupCreate:  {params: BackupParts{}, base: func(f *contractFuzz) any { return BackupParts{Parts: f.parts()} }},
		MethodBackupInspect: {params: BackupText{}, base: backupText},
		MethodBackupRestore: {params: BackupRestore{}, sanitize: sanitizeRestore, base: func(f *contractFuzz) any {
			return BackupRestore{Text: f.pool.backup, Parts: f.parts()}
		}},
	}
}

// contractPool is what the data holds now, for inputs that name it.
type contractPool struct {
	ids                               []string
	storeGroups, manual, storeServers []string
	netGroups, subs, netServers       []string
	presets, copies, otp, conns       []string
	profiles                          map[string]profile.Profile
	backup, presetLink, deepLink      string
}

type contractFuzz struct {
	r          *rand.Rand
	store, net *Engine
	provider   string
	dead       string
	seedLinks  string
	words      []string
	oddities   []string
	pool       contractPool
	refills    int
}

func (f *contractFuzz) chance(p float64) bool { return f.r.Float64() < p }

func (f *contractFuzz) pickID(list []string) string {
	if len(list) == 0 || f.chance(0.1) {
		return f.str()
	}
	return list[f.r.IntN(len(list))]
}

func (f *contractFuzz) someIDs(list []string) []string {
	ids := make([]string, 1+f.r.IntN(3))
	for i := range ids {
		ids[i] = f.pickID(list)
	}
	return ids
}

func (f *contractFuzz) parts() []string {
	var parts []string
	for _, part := range backupParts {
		if f.chance(0.6) {
			parts = append(parts, part)
		}
	}
	return parts
}

var robustOdd = []string{"Привет, мир", "日本語のサーバー", "🇩🇪 Germany 😀", "\u0000", "a\u0000b", "\u202eevil", "\ufeffbom",
	"line\nbreak", "\r\n", "\t", `\`, `"`, "'", "\xff\xfe", "%00", "%zz", "../../windows", `C:\Windows\System32`, "<script>",
	"     ", "\u2028", "ǅ", "İ", "ﬀ", "-", "0x10", "1e3", "true", "null", "{}", "[]"}

var robustPieces = []string{"%", "%2F", "#", "?", "&", "=", "/", ":", "@", "[", "]", "::", "\x00", "\n", "\r\n", " ", "\t", "é",
	"😀", "\u202e", "..", `\`, `"`, "'", "0", "-1", "99999999999", "==", "+", "*", ",", ";", "|", "localhost", "127.0.0.1",
	"[::1]", "%%", "\xff"}

var robustNumbers = []string{"0", "1", "-1", "2", "7", "100", "3600", "65535", "65536", "2147483647", "-2147483648",
	"9223372036854775807", "-9223372036854775808", "18446744073709551615", "1e308", "-1e308", "1e400", "0.5", "-0", "1.5e3",
	"4.9e-324", "123456789012345678901234567890"}

func (f *contractFuzz) wordList() []string {
	return []string{"simple", "advanced", "en", "ru", "off", "connected", "always", "normal", "large", "wireguard", "masque",
		"h2", "h3", "proxy", "direct", "block", "warp", "endpoint", "dns", "suffix", "ip", "domain", "keyword", "regex",
		"processName", "processPath", "processPathRegex", "port", "protocol", "network", "ipVersion", "sourceIP", "private",
		"totp", "hotp", "SHA1", "SHA256", "SHA512", "profiles", "routing", "settings", "otp", "subscription", "manual",
		"hours", "provider", "never", "auto", "on", "1.1", "1.2", "1.3", "full", "download", "upload", "country", "ipv4",
		"ipv6", "smux", "yamux", "h2mux", "local", "tcp", "udp", "tls", "http", "quic", "4", "6", "Ctrl+K", "Ctrl+Alt+P",
		"F7", "Win+Shift+X", "Alt+", "trojan", "vless", "vmess", "shadowsocks", "chain", "custom", "openvpn", "all",
		"hardened", "poros-backup", "poros-preset", "30s", "8MB", "hwid=abc, os=Windows, osVersion=10, model=PC",
		trojanTo(f.dead, "Trojan"), xhttpTo(f.dead, "XHTTP"), ssTo(f.dead, "SS"), vless("de.example.com", "Germany"),
		trojanLink("fi.example.com", "Finland"), f.provider, "https://example.invalid/sub", "http://example.invalid:8080",
		"poros://addsub/" + base64.RawURLEncoding.EncodeToString([]byte(f.provider)), "poros://add/", "poros://route/",
		"otpauth://totp/A:b?secret=JBSWY3DPEHPK3PXP&issuer=A", "otpauth://hotp/C?secret=KRSXG5BAMFZXG5BA&counter=1",
		"otpauth-migration://offline?data=CjEKCkhlbGxvId6tvu8SGEV4YW1wbGU6YWxpY2VAZ29vZ2xlLmNvbRoHRXhhbXBsZTAC",
		"JBSWY3DPEHPK3PXP", "1.1.1.1", "https://1.1.1.1/dns-query", "tls://8.8.8.8", "10.0.0.0/8", "fe80::/10", "::1",
		"172.19.1.1/24", "*.example.com", "example.com", ".example.com", "8000-8080", "443", "0-65535",
		`{"servers":[{"type":"udp","server":"1.1.1.1"}]}`, `{"kind":"poros-backup","v":1,"parts":["settings"]}`,
		`{"type":"direct","tag":"x"}`, `{"outbounds":[]}`, `^.*$`, `(a+)+$`, "example.org\nexample.net",
		"suffix:example.org\nip:10.0.0.0/8\nnonsense", f.dead}
}

func rawOddities() []string {
	return []string{"", " ", "null", "true", "false", "0", "-1", "1e999", `"text"`, "[]", "[null]", "{}", "{", "}", "]",
		`{"id":`, `{"id":"a","id":"b"}`, `{"ID":"x"}`, `{"Id":null}`, "\xef\xbb\xbf{}", "{} {}", "{}x", "/* */{}", "NaN",
		`{"a":NaN}`, "\x00", `{"ids":"x"}`, `{"ids":{"a":1}}`, `{"text":1}`, `{"parts":[1,2]}`, `{"id":{"$gt":""}}`,
		`"\ud800"`, `{"id":"\ud800\udc00"}`, `{"text":"\u0000"}`,
		strings.Repeat("[", 20000) + strings.Repeat("]", 20000),
		strings.Repeat(`{"a":`, 5000) + "1" + strings.Repeat("}", 5000),
		"[" + strings.Repeat("0,", 100000) + "0]",
		`{"text":"` + strings.Repeat("A", 2<<20) + `"}`,
		`{"ids":[` + strings.Repeat(`"x",`, 50000) + `"x"]}`,
	}
}

func (f *contractFuzz) str() string {
	switch n := f.r.IntN(100); {
	case n < 15:
		return ""
	case n < 40 && len(f.pool.ids) > 0:
		return f.pool.ids[f.r.IntN(len(f.pool.ids))]
	case n < 65:
		return f.words[f.r.IntN(len(f.words))]
	case n < 75:
		return robustOdd[f.r.IntN(len(robustOdd))]
	case n < 78:
		return f.huge()
	case n < 90:
		return f.mutateString(f.words[f.r.IntN(len(f.words))])
	}
	runes := make([]rune, 1+f.r.IntN(40))
	for i := range runes {
		runes[i] = rune(f.r.IntN(0x3000))
	}
	return string(runes)
}

func (f *contractFuzz) huge() string {
	size := []int{4 << 10, 64 << 10, 256 << 10, 1 << 20}[f.r.IntN(4)]
	piece := f.words[f.r.IntN(len(f.words))] + " "
	return strings.Repeat(piece, size/len(piece)+1)[:size]
}

func (f *contractFuzz) number() json.Number {
	if f.chance(0.5) {
		return json.Number(robustNumbers[f.r.IntN(len(robustNumbers))])
	}
	return json.Number(strconv.Itoa(f.r.IntN(2000) - 100))
}

func (f *contractFuzz) length() int {
	switch n := f.r.IntN(100); {
	case n < 30:
		return 0
	case n < 97:
		return 1 + f.r.IntN(3)
	}
	return 20 + f.r.IntN(500)
}

// anyValue is JSON of any kind: what a client of the wrong shape sends.
func (f *contractFuzz) anyValue(depth int) any {
	kinds := 8
	if depth > 3 {
		kinds = 5
	}
	switch f.r.IntN(kinds) {
	case 0:
		return nil
	case 1:
		return f.chance(0.5)
	case 2:
		return f.number()
	case 3, 4:
		return f.str()
	case 5:
		object := map[string]any{}
		for range f.r.IntN(4) {
			object[f.str()] = f.anyValue(depth + 1)
		}
		return object
	case 6:
		list := make([]any, f.r.IntN(4))
		for i := range list {
			list[i] = f.anyValue(depth + 1)
		}
		return list
	}
	nested := f.anyValue(depth + 1)
	for range 10 + f.r.IntN(300) {
		if f.chance(0.5) {
			nested = []any{nested}
		} else {
			nested = map[string]any{"a": nested}
		}
	}
	return nested
}

func jsonName(field reflect.StructField) string {
	if !field.IsExported() {
		return ""
	}
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	switch name {
	case "-":
		return ""
	case "":
		return field.Name
	}
	return name
}

var rawMessageType = reflect.TypeFor[json.RawMessage]()

// typed is JSON mostly of the shape of t: fields left out, values of the wrong kind now and then, a field the type
// does not have.
func (f *contractFuzz) typed(t reflect.Type, depth int) any {
	wrong := 0.01
	if depth == 0 {
		wrong = 0.05
	}
	if depth > 8 || f.chance(wrong) || t == rawMessageType {
		return f.anyValue(depth)
	}
	switch t.Kind() {
	case reflect.Pointer:
		if f.chance(0.25) {
			return nil
		}
		return f.typed(t.Elem(), depth)
	case reflect.Struct:
		object := map[string]any{}
		for i := range t.NumField() {
			field := t.Field(i)
			name := jsonName(field)
			share := 0.65
			if field.Type.Kind() == reflect.Pointer {
				share = 0.15
			}
			if name != "" && f.chance(share) {
				object[name] = f.typed(field.Type, depth+1)
			}
		}
		if f.chance(0.05) {
			object[f.str()] = f.anyValue(depth + 1)
		}
		return object
	case reflect.Slice, reflect.Array:
		list := make([]any, f.length())
		for i := range list {
			list[i] = f.typed(t.Elem(), depth+1)
		}
		return list
	case reflect.Map:
		object := map[string]any{}
		for range f.length() {
			object[f.str()] = f.typed(t.Elem(), depth+1)
		}
		return object
	case reflect.String:
		return f.str()
	case reflect.Bool:
		return f.chance(0.5)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return f.number()
	}
	return f.anyValue(depth)
}

func (f *contractFuzz) mutateString(s string) string {
	b := []byte(s)
	for range 1 + f.r.IntN(3) {
		switch f.r.IntN(9) {
		case 0:
			b = slices.Insert(b, f.r.IntN(len(b)+1), []byte(robustPieces[f.r.IntN(len(robustPieces))])...)
		case 1:
			if len(b) > 0 {
				i := f.r.IntN(len(b))
				b = slices.Delete(b, i, i+f.r.IntN(len(b)-i+1))
			}
		case 2:
			if len(b) > 0 {
				b[f.r.IntN(len(b))] = byte(f.r.IntN(256))
			}
		case 3:
			if len(b) > 0 {
				b = b[:f.r.IntN(len(b))]
			}
		case 4:
			b = append(b, b...)
		case 5:
			b = []byte(strings.ToUpper(string(b)))
		case 6:
			b = append([]byte(f.words[f.r.IntN(len(f.words))]), b...)
		case 7:
			if len(b) > 1 {
				i, j := f.r.IntN(len(b)), f.r.IntN(len(b))
				b[i], b[j] = b[j], b[i]
			}
		case 8:
			b = append(b, robustPieces[f.r.IntN(len(robustPieces))]...)
		}
	}
	return string(b[:min(len(b), 4<<20)])
}

// mutateJSONText changes JSON inside a string, such as a backup, rather than the string as a whole.
func (f *contractFuzz) mutateJSONText(text string, depth int) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	tree := f.tree([]byte(trimmed))
	if tree == nil {
		return "", false
	}
	for range 1 + f.r.IntN(3) {
		tree = f.mutate(tree, depth+1)
	}
	data, err := json.Marshal(tree)
	return string(data), err == nil
}

// mutate changes one place of a JSON value: deep in it mostly, the value as a whole now and then.
func (f *contractFuzz) mutate(v any, depth int) any {
	switch x := v.(type) {
	case map[string]any:
		keys := slices.Sorted(maps.Keys(x))
		if len(keys) > 0 && f.chance(0.75) {
			k := keys[f.r.IntN(len(keys))]
			switch f.r.IntN(8) {
			case 0:
				delete(x, k)
			case 1:
				x[k] = nil
			case 2:
				// Go matches the names of fields without regard to case.
				x[strings.ToUpper(k)] = f.mutate(x[k], depth+1)
			default:
				x[k] = f.mutate(x[k], depth+1)
			}
			return x
		}
		if f.chance(0.6) {
			x[f.str()] = f.anyValue(depth + 1)
			return x
		}
	case []any:
		if len(x) > 0 && f.chance(0.6) {
			i := f.r.IntN(len(x))
			x[i] = f.mutate(x[i], depth+1)
			return x
		}
		switch f.r.IntN(5) {
		case 0:
			return append(x, f.anyValue(depth+1))
		case 1:
			if len(x) > 0 {
				return x[:len(x)-1]
			}
			return x
		case 2:
			if len(x) > 0 {
				for range 1 + f.r.IntN(50) {
					x = append(x, x[f.r.IntN(len(x))])
				}
			}
			return x
		case 3:
			return []any{}
		}
	case string:
		if f.chance(0.25) {
			if inner, ok := f.mutateJSONText(x, depth); ok {
				return inner
			}
		}
		if f.chance(0.7) {
			return f.mutateString(x)
		}
		if f.chance(0.5) {
			return f.str()
		}
	case json.Number:
		if f.chance(0.7) {
			return f.number()
		}
	case bool:
		if f.chance(0.7) {
			return !x
		}
	}
	return f.anyValue(depth)
}

// tree reads JSON into maps, lists and json.Number; nil when it is no JSON.
func (f *contractFuzz) tree(data []byte) any {
	var out any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&out) != nil {
		return nil
	}
	return out
}

func (f *contractFuzz) encode(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return data
}

// maxInput keeps the params well below the 64 MB line of the pipe, beyond which the JSON-RPC server reads nothing:
// the mutations of a large value multiply it.
const maxInput = 8 << 20

// input is the params of one call.
func (f *contractFuzz) input(m contractMethod) []byte {
	for {
		if raw := f.anyInput(m); len(raw) <= maxInput {
			return raw
		}
	}
}

func (f *contractFuzz) anyInput(m contractMethod) []byte {
	n := f.r.IntN(100)
	switch {
	case n < 6:
		return []byte(f.oddities[f.r.IntN(len(f.oddities))])
	case n < 9:
		b := make([]byte, f.r.IntN(64))
		for i := range b {
			b[i] = byte(f.r.IntN(256))
		}
		return b
	case m.params == nil:
		return f.encode(f.anyValue(0))
	case n < 40 || m.base == nil:
		return f.encode(f.typed(reflect.TypeOf(m.params), 0))
	}
	data, err := json.Marshal(m.base(f))
	if err != nil {
		return []byte("{}")
	}
	tree := f.tree(data)
	for range f.r.IntN(4) {
		tree = f.mutate(tree, 0)
	}
	return f.encode(tree)
}

// refresh reads what the data holds now.
func (f *contractFuzz) refresh() {
	p := contractPool{profiles: map[string]profile.Profile{}}
	if groups, err := f.store.Groups(); err == nil {
		for _, g := range groups {
			p.storeGroups = append(p.storeGroups, g.ID)
			if g.Kind == GroupManual {
				p.manual = append(p.manual, g.ID)
			}
		}
	}
	if servers, err := f.store.Servers(ServersQuery{}); err == nil {
		for _, s := range servers {
			p.storeServers = append(p.storeServers, s.ID)
		}
	}
	if groups, err := f.net.Groups(); err == nil {
		for _, g := range groups {
			p.netGroups = append(p.netGroups, g.ID)
			if g.Kind == GroupSubscription {
				p.subs = append(p.subs, g.ID)
			}
		}
	}
	if servers, err := f.net.Servers(ServersQuery{}); err == nil {
		for _, s := range servers {
			p.netServers = append(p.netServers, s.ID)
		}
	}
	if list, err := f.store.Presets(); err == nil {
		for _, preset := range list {
			p.presets = append(p.presets, preset.ID)
			if !preset.Builtin {
				p.copies = append(p.copies, preset.ID)
			}
		}
	}
	if list, err := f.store.OTPCodes(); err == nil {
		for _, code := range list {
			p.otp = append(p.otp, code.ID)
		}
	}
	if list, err := f.net.Connections(); err == nil {
		for _, c := range list.Connections {
			p.conns = append(p.conns, c.ID)
		}
	}
	for range min(6, len(p.storeServers)) {
		id := p.storeServers[f.r.IntN(len(p.storeServers))]
		if server, err := f.store.Server(ByID{ID: id}); err == nil {
			p.profiles[id] = server.Profile
		}
	}
	if file, err := f.store.CreateBackup(BackupParts{Parts: backupParts}); err == nil {
		p.backup = file.Text
	}
	if len(p.copies) > 0 {
		if link, err := f.store.ExportPreset(p.copies[f.r.IntN(len(p.copies))]); err == nil {
			p.presetLink = link.Link
		}
	}
	if len(p.storeServers) > 0 {
		if links, err := f.store.DeepLinks(ServerIDs{IDs: []string{p.storeServers[f.r.IntN(len(p.storeServers))]}}); err == nil && len(links.Links) > 0 {
			p.deepLink = links.Links[0]
		}
	}
	p.ids = slices.Concat(p.storeGroups, p.storeServers, p.netGroups, p.netServers, p.presets, p.otp, p.conns)
	f.pool = p
}

// replenish brings back servers, a preset and a code when the calls removed them, so that inputs keep naming data.
func (f *contractFuzz) replenish(t *testing.T) {
	t.Helper()
	changed := false
	if len(f.pool.storeServers) < 4 || len(f.pool.manual) == 0 {
		if _, err := f.store.AddServers(AddServers{NewGroup: "Refill", Text: f.seedLinks}); err != nil {
			t.Errorf("refill the servers: %v", err)
		}
		changed = true
	}
	if len(f.pool.copies) == 0 {
		f.refills++
		if _, err := f.store.SavePreset(PresetCopy{BasedOn: standardPreset, Name: fmt.Sprintf("Refill %d", f.refills),
			Rules: []PresetRule{rule(CondSuffix, "example.org", ActionDirect)}}); err != nil {
			t.Logf("refill a preset: %v", err)
		}
		changed = true
	}
	if len(f.pool.otp) == 0 {
		if _, err := f.store.SaveOTP(totp("Refill", "JBSWY3DPEHPK3PXP")); err != nil {
			t.Errorf("refill a code: %v", err)
		}
		changed = true
	}
	if changed {
		f.refresh()
	}
}

// seed gives the engines data: the store servers of many kinds, presets, codes, settings of its own; the network
// engine servers that lead to the port that closes at once, a subscription from the local provider, and settings
// that keep everything else on loopback.
func (f *contractFuzz) seed(t *testing.T) {
	t.Helper()
	for _, e := range []*Engine{f.store, f.net} {
		if _, err := e.AddSubscription(t.Context(), AddSubscription{URL: f.provider}); err != nil {
			t.Fatalf("AddSubscription: %v", err)
		}
	}
	f.seedLinks = strings.Join([]string{trojanTo(f.dead, "Trojan"), xhttpTo(f.dead, "XHTTP"), ssTo(f.dead, "SS")}, "\n")
	mine, err := f.store.AddServers(AddServers{NewGroup: "Mine", Text: f.seedLinks + "\n" + vless("de.example.com", "Germany") + "\n" +
		trojanLink("fi.example.com", "Finland")})
	if err != nil || len(mine.IDs) < 4 {
		t.Fatalf("AddServers: %+v, %v", mine, err)
	}
	ovpn, oc, _ := vpnProfiles(t, f.store)
	if _, err = f.store.SaveServer(SaveServer{GroupID: mine.Group.ID, Profile: chainProfile(mine.IDs[0], mine.IDs[3])}); err != nil {
		t.Fatalf("a chain: %v", err)
	}
	simple, err := f.store.SavePreset(PresetCopy{BasedOn: standardPreset, Name: "Simple", Rules: []PresetRule{
		rule(CondSuffix, "example.org", ActionDirect), rule(CondIP, "10.0.0.0/8", ActionBlock), rule(CondProcessName, "game.exe", ActionProxy)}})
	if err != nil {
		t.Fatalf("a simple preset: %v", err)
	}
	office := rule(CondSuffix, "office.example", ActionEndpoint)
	office.Endpoint = oc
	if _, err = f.store.SavePreset(PresetCopy{BasedOn: standardPreset, Name: "Advanced", Advanced: true, Endpoints: []string{ovpn, oc},
		Rules: []PresetRule{office, rule(CondPort, "8000-8080", ActionProxy), rule(CondKeyword, "tracker", ActionBlock)}}); err != nil {
		t.Fatalf("an advanced preset: %v", err)
	}
	if _, err = f.store.SaveOTP(totp("GitHub", "JBSWY3DPEHPK3PXP")); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.SaveOTP(OTPEntry{Name: "Office", Secret: "KRSXG5BAMFZXG5BA", Kind: OTPKindHOTP, Algorithm: OTPSHA1, Digits: 6, Period: 30}); err != nil {
		t.Fatal(err)
	}
	settings, _ := f.store.Settings()
	settings.Server, settings.Preset, settings.CheckIP = mine.IDs[0], simple.ID, false
	if _, err = f.store.SetSettings(settings); err != nil {
		t.Fatal(err)
	}

	dead, err := f.net.AddServers(AddServers{NewGroup: "Dead", Text: f.seedLinks})
	if err != nil || len(dead.IDs) != 3 {
		t.Fatalf("AddServers: %+v, %v", dead, err)
	}
	settings, _ = f.net.Settings()
	settings.Server, settings.CheckIP, settings.WARPHosts, settings.SpeedTestURL = dead.IDs[0], false, f.dead, "http://"+f.dead+"/down"
	if _, err = f.net.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	f.words, f.oddities = f.wordList(), rawOddities()
	f.refresh()
}

type contractStats struct {
	calls, results, params, bare int
	reasons                      map[string]int
	samples                      []string
}

// Every method of the contract, called through the decoding of the JSON-RPC server with random and broken params:
// none panics or hangs, each answers with JSON or an error, and an error the interface gets carries a reason of
// api.go. Methods that fail without a reason are listed rather than failed: whether a reason is due is for a person
// to judge. Afterwards the data holds and opens again.
func TestRobustContract(t *testing.T) {
	robustOnHost(t)
	checkLeaksAfter(t)
	seed := robustSeed(t)
	rounds := 30
	if robustStress() {
		rounds = 1000
	}
	reasons := apiConstants(t, "Reason")
	constants := apiConstants(t, "Method")

	dead := closerServer(t)
	page := webPage(t)
	provider := newProvider(t)
	provider.serve(http.StatusOK, trojanTo(dead, "Sub%20A"), trojanTo(dead, "Sub%20B"), xhttpTo(dead, "Sub%20C"))
	allowed := []string{hostOf(t, provider.url), dead}
	servers, client := dohServers, dohClient
	t.Cleanup(func() { dohServers, dohClient = servers, client })
	dohServers, dohClient = []string{"http://" + dead + "/dns-query"}, func() *http.Client { return loopbackClient(dead) }

	storeDir := t.TempDir()
	store, closeStore := openRobust(t, storeDir, page, allowed...)
	netEngine, closeNet := openRobust(t, t.TempDir(), page, allowed...)
	f := &contractFuzz{r: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)), store: store, net: netEngine, provider: provider.url, dead: dead}
	f.seed(t)

	methods := contractMethods()
	storeHandlers, netHandlers := store.Handlers(), netEngine.Handlers()
	for value, name := range constants {
		if _, ok := methods[value]; !ok {
			t.Errorf("%s (%s) is not in contractMethods", name, value)
		}
		if storeHandlers[value] == nil {
			t.Errorf("%s (%s) has no handler", name, value)
		}
	}
	for method := range storeHandlers {
		if constants[method] == "" {
			t.Errorf("the handler %s is no method of api.go", method)
		}
	}
	for method := range methods {
		if constants[method] == "" {
			t.Errorf("contractMethods has %s, which api.go does not", method)
		}
	}
	names := slices.Sorted(maps.Keys(methods))
	for _, name := range names {
		if skip := methods[name].skip; skip != "" {
			t.Logf("skipped %s: %s", name, skip)
		}
	}
	t.Logf("%d methods and %d reasons in api.go, %d rounds", len(constants), len(reasons), rounds)

	stats := map[string]*contractStats{}
	calls := 0
	began := time.Now()
calling:
	for round := range rounds {
		order := slices.Clone(names)
		f.r.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		for _, name := range order {
			m := methods[name]
			if m.skip != "" {
				continue
			}
			raw := f.input(m)
			if m.sanitize != nil {
				raw = m.sanitize(raw)
			}
			handler := storeHandlers[name]
			if m.onNet {
				handler = netHandlers[name]
			}
			out := callHandler(handler, raw, callLimit)
			calls++
			st := stats[name]
			if st == nil {
				st = &contractStats{reasons: map[string]int{}}
				stats[name] = st
			}
			st.calls++
			switch {
			case out.hung:
				t.Errorf("round %d, call %d: %s did not answer in %s to %s\n%s", round, calls, name, callLimit, shown(raw), goroutineDump())
				break calling
			case out.panicked != nil:
				t.Errorf("round %d, call %d: %s panicked on %s: %v\n%s", round, calls, name, shown(raw), out.panicked, out.stack)
			case out.err == nil:
				st.results++
				if _, err := json.Marshal(out.result); err != nil {
					t.Errorf("%s answered with what is no JSON: %v; input %s", name, err, shown(raw))
				}
			default:
				switch class, detail := classify(out.err, reasons); class {
				case errParams:
					st.params++
				case errReason:
					st.reasons[detail]++
				case errUnknown:
					t.Errorf("%s failed with the reason %q, which api.go does not have; input %s", name, detail, shown(raw))
				case errBare:
					st.bare++
					if len(st.samples) < 3 {
						st.samples = append(st.samples, fmt.Sprintf("%.200s (input %.200s)", detail, shown(raw)))
					}
				}
			}
			if calls%15 == 0 {
				f.refresh()
				f.replenish(t)
			}
		}
	}

	var lines, bare []string
	for _, name := range names {
		st := stats[name]
		if st == nil {
			continue
		}
		var counted []string
		for _, r := range slices.Sorted(maps.Keys(st.reasons)) {
			counted = append(counted, fmt.Sprintf("%s %d", r, st.reasons[r]))
		}
		lines = append(lines, fmt.Sprintf("%-22s %4d calls, %4d results, %4d bad params, %3d bare; %s", name, st.calls, st.results,
			st.params, st.bare, strings.Join(counted, ", ")))
		for _, sample := range st.samples {
			bare = append(bare, name+": "+sample)
		}
	}
	t.Logf("%d calls in %s:\n%s", calls, time.Since(began).Round(time.Millisecond), strings.Join(lines, "\n"))
	if len(bare) > 0 {
		t.Logf("errors without a reason:\n%s", strings.Join(bare, "\n"))
	}

	netEngine.StopConnection()
	waitIdle(t, netEngine, 30*time.Second)
	stopTestsWithin(t, netEngine, 30*time.Second)
	stopTestsWithin(t, store, 30*time.Second)
	robustCheckData(t, store)
	robustCheckData(t, netEngine)
	closeStore()
	closeNet()
	reopened, closeReopened := openRobust(t, storeDir, page)
	robustCheckData(t, reopened)
	closeReopened()
}
