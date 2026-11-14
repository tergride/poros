package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
	"time"

	"PorosCore/config"
)

// hangLimit bounds one input. The fuzzing engine has no time limit of its own, and a reader that hangs on what came
// from outside would hang the core.
const hangLimit = 10 * time.Second

// watchHang crashes the process when an input runs past hangLimit, which keeps the input as a crasher.
func watchHang() (stop func()) {
	timer := time.AfterFunc(hangLimit, func() {
		debug.SetTraceback("all")
		panic("the input ran longer than " + hangLimit.String())
	})
	return func() { timer.Stop() }
}

// failsWith fails a test whose error is not a failure of the core with the reason.
func failsWith(t *testing.T, err error, want string) {
	t.Helper()
	if got := reason(err); got != want {
		t.Fatalf("the error %v has the reason %q, want %q", err, got, want)
	}
}

// presetEngine is an engine with the built-in presets and no data: enough to read and check presets.
func presetEngine(f *testing.F) *Engine {
	presets, err := builtinPresets()
	if err != nil {
		f.Fatal(err)
	}
	return &Engine{presets: presets}
}

// The lists window is text the user types or pastes: whatever its lines, it gives rules a preset may keep, and the
// rules written back into the window read into the same rules without a problem.
func FuzzReadPresetLists(f *testing.F) {
	f.Add(false, "suffix:video.example.com\r\n# work\n\n  suffix:https://Mail.Example.org/inbox  \nmedia.example.net\nSUFFIX:media.example.net\n"+
		"domain:exact.example\nsuffix:video.example.com",
		"ip:192.168.50.0/24\nIP:203.0.113.7\nprocessName:steam.exe\nprocessPath:C:\\Games\\game.exe\nprocessname:steam\n"+
			"suffix:*.video.example.com\nprocessName:STEAM.EXE",
		"suffix:*.ads.example\ngeosite:category-ads\nregex:\\.ads\\.example$\nsuffix:*.\nprocessName:steam.exe\nC:\\Tools\\tool.exe", "")
	f.Add(true, "suffix:a.example\nsuffix:x.a.example", "domain:exact.example\nkeyword:tracker\nregex:\\.ads\\.example$\n"+
		"processPathRegex:(?i)games\nport:8000-8080\nport:443\nprotocol:QUIC\nnetwork:udp\nipVersion:6\nsourceIP:192.168.1.0/24\nprivate:",
		"ip:10.0.0.0/8\nip:10.1.0.0/16\nip:::ffff:10.1.2.3\nport:8000:8010\nip:[2001:db8::1]", "suffix:warp.example\nipVersion:4\nport:0")
	f.Fuzz(func(t *testing.T, advanced bool, proxy, direct, block, warp string) {
		defer watchHang()()
		lists := map[string]string{ActionProxy: proxy, ActionDirect: direct, ActionBlock: block}
		if advanced {
			lists[ActionWARP] = warp
		}
		read, err := ReadPresetLists(PresetListsInput{Advanced: advanced, Lists: lists})
		if err != nil {
			t.Fatalf("the window's own tabs: %v", err)
		}
		for _, p := range read.Problems {
			if p.Reason == "" || p.Line < 1 || !slices.Contains(listActions(advanced), p.Action) {
				t.Fatalf("a problem %+v", p)
			}
		}
		if _, err := checkRules(advanced, nil, read.Rules); err != nil {
			t.Fatalf("the window gives rules a preset refuses: %v\n%+v", err, read.Rules)
		}
		written, err := WritePresetLists(PresetRules{Advanced: advanced, Rules: read.Rules})
		if err != nil {
			t.Fatalf("the rules of the window are not written back: %v", err)
		}
		again, err := ReadPresetLists(PresetListsInput{Advanced: advanced, Rules: read.Rules, Lists: written.Lists})
		if err != nil || len(again.Problems) > 0 || !reflect.DeepEqual(again.Rules, read.Rules) {
			t.Fatalf("the rules\n%+v\nare written as %q, which reads as\n%+v\nwith %+v, %v", read.Rules, written.Lists, again.Rules, again.Problems, err)
		}
	})
}

func presetLinkOf(f *testing.F, link presetLink) string {
	data, err := json.Marshal(link)
	if err != nil {
		f.Fatal(err)
	}
	return presetLinkPrefix + base64.RawURLEncoding.EncodeToString(data)
}

// Links from a browser or a messenger: whatever poros:// or throne:// link comes, ParseLink tells what it adds or why
// not. A preset it brings is checked as its import would, and one that passes is written as its export would and
// reads back the same.
func FuzzOutsideLink(f *testing.F) {
	e := presetEngine(f)
	encode := func(text string) string { return base64.RawURLEncoding.EncodeToString([]byte(text)) }
	for _, link := range []string{
		"poros://AddSub/" + encode("https://sub.example.com/u/8f3a#My%20VPN"),
		"throne://addsub/" + base64.StdEncoding.EncodeToString([]byte("https://vpn.example.org/s/71bd")),
		"poros://addsub/" + encode("vless://id@host:443"),
		"poros://addsub/" + encode("ftp://example.com/x"),
		"poros://addsub/" + encode("http://:80/#%zz"),
		"poros://add/" + encode(`{"type":"trojan","name":"🇩🇪 Home","server":"de.example.com","server_port":443,"trojan":{"password":"p","tls":{"enabled":true}}}`),
		"throne://add/" + encode(`{"type":"trojan","tag":"Throne","server":"th.example.com","server_port":8443,"password":"p"}`),
		"poros://add/" + encode(`{"type":"vless"}`),
		"throne://route/" + encode("{}"),
		"poros://remoteroute/x",
		presetLinkOf(f, presetLink{Kind: presetLinkKind, Version: 1, BasedOn: "all", Name: "Home", DNS: "tls://1.1.1.1",
			Rules: []PresetRule{rule(CondSuffix, "example.org", ActionBlock), rule(CondIP, "10.0.0.0/8", ActionDirect)}}),
		presetLinkOf(f, presetLink{Kind: presetLinkKind, Version: 1, BasedOn: "all", Name: "Advanced", Advanced: true, Default: ActionDirect,
			DirectDNS: "local", DNS: "https://dns.example/dns-query", Rules: []PresetRule{
				{Name: "Games", Action: ActionWARP, Invert: true, Conditions: []RuleCondition{
					{Kind: CondProcessName, Values: []string{"game.exe", "GAME.EXE"}}, {Kind: CondNetwork, Values: []string{"udp"}}}},
				{Action: ActionProxy, Off: true, Conditions: []RuleCondition{{Kind: CondPrivate, Values: []string{}}}},
				rule(CondPort, "8000-8080", ActionBlock), rule(CondPort, "8008", ActionProxy), rule(CondRegex, `\.ads\.example$`, ActionBlock),
			}}),
		"poros://route/" + encode(`{"kind":"poros-preset","v":1,"based_on":"all","name":"X","rules":[{"conditions":[],"action":"proxy","extra":1}]}`),
	} {
		f.Add(link)
	}
	f.Fuzz(func(t *testing.T, link string) {
		defer watchHang()()
		info, err := e.ParseLink(OutsideLink{Link: link})
		if err != nil {
			if reason(err) == "" {
				t.Fatalf("the error %v has no reason", err)
			}
			return
		}
		switch info.Kind {
		case LinkSubscription:
			if err := checkSubscriptionURL(info.URL); err != nil {
				t.Fatalf("the subscription %q: %v", info.URL, err)
			}
		case LinkServer:
			if info.Server == nil || info.Profile == nil || info.Profile.Validate() != nil {
				t.Fatalf("the server link gives %+v", info)
			}
		case LinkPreset:
			checkPresetLink(t, e, *info.Preset)
		default:
			t.Fatalf("a link of kind %q", info.Kind)
		}
	})
}

// checkPresetLink runs what saving the imported copy checks before it is stored, then the copy's export: it must
// read back into the same rules.
func checkPresetLink(t *testing.T, e *Engine, c PresetCopy) {
	t.Helper()
	rules, err := checkRules(c.Advanced, c.Endpoints, c.Rules)
	if err != nil {
		return
	}
	record := copyRecord{BasedOn: c.BasedOn, Name: c.Name, Advanced: c.Advanced, Rules: rules, DNS: strings.TrimSpace(c.DNS),
		DirectDNS: strings.TrimSpace(c.DirectDNS), Default: c.Default, Endpoints: c.Endpoints}
	if _, ok := e.foundation(record.BasedOn); !ok {
		return
	}
	if _, err = e.compose(record); err != nil {
		return
	}
	data, err := json.Marshal(presetLink{Kind: presetLinkKind, Version: 1, BasedOn: record.BasedOn, Name: record.Name,
		Advanced: record.Advanced, Rules: rules, DNS: record.DNS, DirectDNS: record.DirectDNS, Default: record.Default})
	if err != nil {
		t.Fatalf("the preset has no JSON: %v", err)
	}
	exported := presetLinkPrefix + base64.RawURLEncoding.EncodeToString(data)
	back, err := decodePresetLink(exported)
	if err != nil {
		t.Fatalf("the export %q does not read back: %v", exported, err)
	}
	again, err := checkRules(back.Advanced, nil, back.Rules)
	if err != nil || !reflect.DeepEqual(again, rules) {
		t.Fatalf("the rules\n%+v\nare exported as %q, which reads as\n%+v, %v", rules, exported, again, err)
	}
}

// A backup is a file the user picks: whatever its text, backup.inspect tells what it holds or refuses it as a backup.
// Nothing is restored here: a restored kill switch would reach the system proxy of this computer.
func FuzzInspectBackup(f *testing.F) {
	e, err := Open(Config{DataDir: f.TempDir(), Version: "0.1.0"})
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { e.Close() })
	if _, err = e.AddServers(AddServers{NewGroup: "Mine", Text: "trojan://pw@trojan.example.com:443?sni=example.com#Home\n" +
		"vless://" + "b831381d-6324-4d53-ad4f-8cda48b30811" + "@v.example.com:443?security=tls&type=ws&path=%2Fws#WS"}); err != nil {
		f.Fatal(err)
	}
	if _, err = e.SavePreset(PresetCopy{BasedOn: "all", Name: "Home", Rules: []PresetRule{rule(CondSuffix, "example.org", ActionDirect)}}); err != nil {
		f.Fatal(err)
	}
	if _, err = e.SaveOTP(totp("GitHub", "JBSWY3DPEHPK3PXP")); err != nil {
		f.Fatal(err)
	}
	all, err := e.CreateBackup(BackupParts{Parts: []string{BackupProfiles, BackupRouting, BackupSettings, BackupOTP}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(all.Text)
	for _, parts := range [][]string{{BackupSettings}, {BackupOTP, BackupRouting}} {
		some, err := e.CreateBackup(BackupParts{Parts: parts})
		if err != nil {
			f.Fatal(err)
		}
		f.Add(some.Text)
	}
	f.Add(`{"kind":"poros-backup","v":1,"parts":["profiles"],"groups":[{"id":"g","kind":"manual"}],"servers":{"g":[{"id":"s","profile":{}}]}}`)
	f.Add(`{"kind":"poros-backup","v":2}`)
	f.Fuzz(func(t *testing.T, text string) {
		defer watchHang()()
		info, err := e.InspectBackup(BackupText{Text: text})
		if err != nil {
			failsWith(t, err, ReasonInvalidBackup)
			return
		}
		for _, part := range info.Parts {
			if !slices.Contains(backupParts, part) {
				t.Fatalf("the backup has a part %q", part)
			}
		}
	})
}

// The TUN of the settings is text the user types: whatever it holds, the settings take it or refuse it with a
// reason, the ranges kept out read back from their own text, and the TUN of a connection is made of what passed.
func FuzzTunSettings(f *testing.F) {
	f.Add(TunBypassDefault, TunIPv4Default, TunIPv6Default)
	f.Add("# home\r\n192.168.50.1\n  2001:db8::1  \n10.1.2.3/8\n::ffff:10.0.0.0/104", "10.0.0.1/30", "fd00::2/126")
	f.Add("10.0.0.0/8\n0.0.0.0/0", "172.19.1.0/24", "fdfe::1/96")
	f.Add("lan\n::/0", "fdfe::1/96", "172.19.1.1/24")
	f.Add("", " 172.19.1.1/24 ", "fe80::1%eth0/64")
	f.Fuzz(func(t *testing.T, bypass, ipv4, ipv6 string) {
		defer watchHang()()
		ranges, err := bypassOf(bypass)
		if err != nil {
			failsWith(t, err, ReasonInvalidValue)
		} else {
			lines := make([]string, len(ranges))
			for i, r := range ranges {
				if !r.IsValid() || r.Bits() == 0 || r != r.Masked() {
					t.Fatalf("the range %v of %q", r, bypass)
				}
				lines[i] = r.String()
			}
			if back, err := bypassOf(strings.Join(lines, "\n")); err != nil || !slices.Equal(back, ranges) {
				t.Fatalf("the ranges %v of %q are written as %q, which reads as %v, %v", ranges, bypass, lines, back, err)
			}
		}
		s := DefaultSettings()
		s.TunBypass, s.TunIPv4, s.TunIPv6 = bypass, ipv4, ipv6
		if err := validateTun(s); err != nil {
			failsWith(t, err, ReasonInvalidValue)
			return
		}
		tun := tunOf(s)
		if !tun.Address4.Addr().Is4() || !tun.Address6.Addr().Is6() || tun.Address4.Bits() > 30 || tun.Address6.Bits() > 126 {
			t.Fatalf("the TUN of %q and %q: %+v", ipv4, ipv6, tun)
		}
	})
}

// An own dns object is JSON the user writes: whatever it holds, dns.check takes it or tells why not, without a
// panic of the core or of sing-box.
func FuzzDNSObject(f *testing.F) {
	for _, object := range []string{
		`{"servers":[{"type":"https","tag":"remote","server":"1.1.1.1","detour":"proxy"}],"final":"remote"}`,
		`{"servers":[{"type":"https","tag":"remote","server":"1.1.1.1","detour":"proxy"},{"type":"local","tag":"system"}],` +
			`"rules":[{"domain_suffix":["lan"],"server":"system"},{"type":"logical","mode":"or","rules":[{"query_type":["AAAA"]}],` +
			`"action":"reject"}],"final":"remote","strategy":"ipv4_only","cache_capacity":1024}`,
		`{"servers":[{"type":"fakeip","tag":"fake","inet4_range":"198.18.0.0/15"},{"type":"tls","tag":"t","server":"dns.example",` +
			`"domain_resolver":"fake"}],"rules":[{"query_type":["A"],"server":"fake"}]}`,
		`{"servers":[{"type":"hosts","tag":"h","predefined":{"nas.home":["192.168.1.20"]}}]}`,
		`{"servers":[{"type":"local","tag":"local","detour":"proxi"}]}`,
		`{"servers":[{"type":"local","tag":"local"}]}`,
		`{"final":"remote"}`,
		`{"servers":[{"type":"udp","tag":"u","server":"8.8.8.8","server_port":0}],"rules":[{"rules":[{"server":"x"}]}]}`,
		`[]`, `{}`, ` `, `null`,
	} {
		f.Add(object)
	}
	f.Fuzz(func(t *testing.T, object string) {
		defer watchHang()()
		if err := checkDNSObject(object); err != nil {
			failsWith(t, err, ReasonInvalidValue)
		}
	})
}

// The DNS settings the user types besides the object: servers, ready answers and the subnet of ECS. Answers that are
// taken read back from their own text.
func FuzzDNSText(f *testing.F) {
	f.Add("tls://1.1.1.1", "nas.home 192.168.1.20\n# note\nNAS2.home. ::ffff:192.168.1.21 2001:db8::1", "203.0.113.7")
	f.Add("https://dns.example/dns-query", "nas.home", "203.0.113.0/33")
	f.Add("dns.example", "a/b 1.2.3.4\na 1.2.3.4\nA. 5.6.7.8", "2001:db8::/48")
	f.Add("udp://[2001:db8::53]:5353", "x 1.2.3.4%eth0", "")
	f.Add("tls://user@dns.example:0?x#y", "", "fe80::1%2")
	f.Fuzz(func(t *testing.T, server, answers, subnet string) {
		defer watchHang()()
		_ = config.CheckDNSServer(server, false)
		_ = config.CheckDNSServer(server, true)
		_, _ = clientSubnet(subnet)
		parsed, err := config.ParseAnswers(answers)
		if err != nil {
			return
		}
		var text strings.Builder
		for _, a := range parsed {
			text.WriteString(a.Name)
			for _, addr := range a.Addresses {
				text.WriteString(" " + addr.String())
			}
			text.WriteString("\n")
		}
		if back, err := config.ParseAnswers(text.String()); err != nil || !reflect.DeepEqual(back, parsed) {
			t.Fatalf("the answers %+v are written as %q, which reads as %+v, %v", parsed, text.String(), back, err)
		}
	})
}

// feedTransport answers every request with the body of its path, without a network.
type feedTransport map[string][]byte

func (bodies feedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	body, found := bodies[r.URL.Path]
	status := http.StatusOK
	if !found {
		status = http.StatusNotFound
	}
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: http.Header{}, Request: r,
		Body: io.NopCloser(bytes.NewReader(body))}, nil
}

// latest.json and the list of releases come from the network: whatever they hold, the check takes a release whose
// archive is a file of its own next to the feed, or fails.
func FuzzUpdateFeed(f *testing.F) {
	sum := strings.Repeat("ab", 32)
	f.Add([]byte(`{"version":"0.1.1","file":"Poros-0.1.1-windows-amd64.zip","size":1024,"sha256":"`+sum+`"}`),
		[]byte(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry><link rel="alternate" type="text/html" `+
			`href="https://github.com/tergride/poros/releases/tag/v0.1.1"/></entry><entry><link href="https://github.com/tergride/poros/releases/tag/v0.2.0"/>`+
			`</entry><entry><link href="https://github.com/tergride/poros/releases/tag/v0.10.0-beta"/></entry></feed>`))
	f.Add([]byte(`{"version":"0.1.1\n","file":"Poros-0.1.1\n-windows-amd64.zip","size":-1,"sha256":"`+strings.ToUpper(sum)+`"}`),
		[]byte(`<feed><entry><link href="/releases/tag/v99999999999999999999.0.0"/></entry></feed>`))
	f.Add([]byte(`{"version":"1.0.0","file":"../Poros-1.0.0-windows-amd64.zip","size":536870913,"sha256":""}`), []byte(`<feed><entry>`))
	f.Fuzz(func(t *testing.T, feed, releases []byte) {
		defer watchHang()()
		e := &Engine{userAgent: "Poros", client: &http.Client{Transport: feedTransport{"/latest.json": feed, "/releases.atom": releases}}}
		e.upd.releasesURL = "http://updates.test/releases.atom"
		ctx := context.Background()
		if version, err := e.newestRelease(ctx); err == nil && version != "" && !versionPattern.MatchString(version) {
			t.Fatalf("the newest release %q", version)
		}
		release, err := e.fetchFeed(ctx, "http://updates.test/latest.json")
		if err != nil {
			return
		}
		if release == nil || !versionPattern.MatchString(release.Version) || filepath.Base(release.File) != release.File ||
			strings.ContainsAny(release.File, `/\:`) || release.Size <= 0 || newerVersion(release.Version, release.Version) {
			t.Fatalf("the release %+v", release)
		}
		link, err := archiveURL(*release, "http://updates.test/feed/latest.json")
		if err != nil {
			t.Fatalf("the archive of %+v: %v", release, err)
		}
		if u, err := url.Parse(link); err != nil || u.Host != "updates.test" || u.Path != "/feed/"+release.File {
			t.Fatalf("the archive of %+v is at %q", release, link)
		}
		if link, err = archiveURL(*release, releasesGitHub+"latest/download/latest.json"); err != nil ||
			link != releasesGitHub+"download/v"+release.Version+"/"+release.File {
			t.Fatalf("the archive of %+v on GitHub is at %q, %v", release, link, err)
		}
	})
}
