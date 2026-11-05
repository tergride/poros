package profile

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

func names(profiles []Profile) []string {
	var out []string
	for _, p := range profiles {
		out = append(out, p.Name)
	}
	return out
}

func mustParseDocument(t *testing.T, body string) Document {
	t.Helper()
	d, err := ParseDocument([]byte(body))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	return d
}

func TestParseDocumentLinkList(t *testing.T) {
	body := "\xef\xbb\xbf#profile-title: Provider\r\n" +
		"Срок действия: до 2026-12-01\r\n" +
		"vless://" + testUUID + "@a.example.com:443?security=tls#A\r\n" +
		"\r\n" +
		"wg://key@198.51.100.7:51820#WG\r\n" +
		"trojan://@t.example.com#Broken\r\n" +
		"https://t.me/provider\r\n" +
		"ss://YWVzLTI1Ni1nY206cHc@ss.example.com:8388#SS\u00a0\r\n"
	d := mustParseDocument(t, body)
	if d.Format != FormatLinks {
		t.Fatalf("format %q", d.Format)
	}
	if got := names(d.Profiles); !reflect.DeepEqual(got, []string{"A", "SS"}) {
		t.Fatalf("notes, comments and the provider's site are not servers: got %q", got)
	}
	want := []Skipped{
		{Index: 5, Name: "WG", Unsupported: true, Reason: "wg:// links are not supported"},
		{Index: 6, Name: "Broken", Reason: "the Trojan password is missing"},
	}
	if !reflect.DeepEqual(d.Skipped, want) {
		t.Fatalf("skipped\n got %+v\nwant %+v", d.Skipped, want)
	}
}

func TestParseDocumentBase64(t *testing.T) {
	list := "vless://" + testUUID + "@a.example.com:443#A\nss://YWVzLTI1Ni1nY206cHc@ss.example.com:8388#B\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(list))
	var wrapped strings.Builder
	for i := 0; i < len(encoded); i += 76 {
		wrapped.WriteString(encoded[i:min(i+76, len(encoded))] + "\r\n")
	}
	for name, body := range map[string]string{
		"one line":           encoded,
		"url alphabet":       strings.TrimRight(base64.URLEncoding.EncodeToString([]byte(list)), "="),
		"wrapped at 76":      wrapped.String(),
		"one blob per line":  encoded + "\n" + base64.StdEncoding.EncodeToString([]byte("trojan://pw@t.example.com#C")),
		"base64 inside list": "# servers\n" + encoded + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			d := mustParseDocument(t, body)
			want := []string{"A", "B"}
			if name == "one blob per line" {
				want = append(want, "C")
			}
			if got := names(d.Profiles); !reflect.DeepEqual(got, want) || len(d.Skipped) != 0 {
				t.Fatalf("got %q, skipped %+v", got, d.Skipped)
			}
		})
	}
}

func TestParseDocumentNestedLineTakesItsIndex(t *testing.T) {
	inner := base64.StdEncoding.EncodeToString([]byte("vless://" + testUUID + "@a.example.com#A\nvless://@b.example.com#Bad"))
	d := mustParseDocument(t, "#note\n"+inner)
	if len(d.Profiles) != 1 || len(d.Skipped) != 1 || d.Skipped[0].Index != 2 || d.Skipped[0].Name != "Bad" {
		t.Fatalf("profiles %q, skipped %+v", names(d.Profiles), d.Skipped)
	}
}

func TestParseDocumentSIP008(t *testing.T) {
	body := `{"version": 1, "servers": [
		{"remarks": "S1", "server": "[2001:db8::1]", "server_port": 8388, "password": "pw", "method": "AES-256-GCM",
		 "plugin": "simple-obfs", "plugin_opts": "obfs=http"},
		{"remarks": "S2", "server": "s2.example.com", "server_port": "8388"},
		{"server": "s3.example.com", "server_port": 8388, "method": "rot13", "password": "pw"}
	]}`
	for name, text := range map[string]string{"plain": body, "base64": base64.StdEncoding.EncodeToString([]byte(body))} {
		t.Run(name, func(t *testing.T) {
			d := mustParseDocument(t, text)
			wantProfiles := []Profile{{Type: TypeShadowsocks, Name: "S1", Server: "2001:db8::1", ServerPort: 8388, Shadowsocks: &Shadowsocks{
				Method: "aes-256-gcm", Password: "pw", Plugin: "obfs-local", PluginOpts: "obfs=http",
			}}}
			if d.Format != FormatSIP008 || !reflect.DeepEqual(d.Profiles, wantProfiles) {
				t.Fatalf("format %q, profiles %s", d.Format, dump(d.Profiles[0]))
			}
			if len(d.Skipped) != 2 || d.Skipped[0].Index != 2 || d.Skipped[0].Name != "" || d.Skipped[0].Unsupported ||
				d.Skipped[1].Index != 3 || !d.Skipped[1].Unsupported {
				t.Fatalf("skipped %+v", d.Skipped)
			}
		})
	}
}

func TestParseDocumentKeepsTheFirstSkipReasons(t *testing.T) {
	d := mustParseDocument(t, strings.Repeat("trojan://@t.example.com#x\n", maxSkipped+500))
	if len(d.Skipped) != maxSkipped || d.SkippedCount != maxSkipped+500 {
		t.Fatalf("kept %d reasons of %d", len(d.Skipped), d.SkippedCount)
	}
}

func TestParseDocumentXrayFragmentIsDropped(t *testing.T) {
	vless := func(tag, dialer string) string {
		return `{"tag": "` + tag + `", "protocol": "vless", "settings": {"vnext": [{"address": "v.example.com", "port": 443,
			"users": [{"id": "` + testUUID + `", "encryption": "none"}]}]},
			"streamSettings": {"network": "tcp", "security": "tls", "sockopt": {"dialerProxy": "` + dialer + `"}}}`
	}
	body := `[{"remarks": "DE", "outbounds": [` + vless("proxy", "fragment") + `,
		{"tag": "fragment", "protocol": "freedom", "settings": {"fragment": {"packets": "tlshello", "length": "100-200"}}},
		{"tag": "direct", "protocol": "freedom"}]},
	{"remarks": "Chain", "outbounds": [` + vless("proxy", "entry") + `, ` + vless("entry", "") + `]},
	{"remarks": "Elsewhere", "outbounds": [` + vless("proxy", "redirect") + `,
		{"tag": "redirect", "protocol": "freedom", "settings": {"redirect": "203.0.113.1:443"}}]}]`
	d := mustParseDocument(t, body)
	if got := names(d.Profiles); !reflect.DeepEqual(got, []string{"DE", "Chain · entry"}) {
		t.Fatalf("a dial through plain freedom is a plain dial: got %q", got)
	}
	if len(d.Skipped) != 2 || d.Skipped[0].Name != "Chain · proxy" || !d.Skipped[0].Unsupported ||
		d.Skipped[1].Name != "Elsewhere" || !d.Skipped[1].Unsupported {
		t.Fatalf("a dial through another server or a redirect is a chain: %+v", d.Skipped)
	}
}

func TestParseDocumentClashAsJSON(t *testing.T) {
	d := mustParseDocument(t, `{"proxies": [{"name": "T", "type": "trojan", "server": "t.example.com", "port": 443, "password": "pw"}]}`)
	if d.Format != FormatClash || !reflect.DeepEqual(names(d.Profiles), []string{"T"}) {
		t.Fatalf("format %q, profiles %q, skipped %+v", d.Format, names(d.Profiles), d.Skipped)
	}
}

func TestParseDocumentBrokenBodies(t *testing.T) {
	for name, tt := range map[string]struct{ body, want string }{
		"json syntax":  {"{\n  \"outbounds\": [\n  }", "at line 3, column 3"},
		"unknown json": {`{"log": {}}`, "neither a sing-box, Xray nor SIP008"},
		"empty list":   {`[]`, "neither sing-box nor Xray"},
		"sip008 shape": {`{"servers": {}}`, "SIP008 servers are not a list"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDocument([]byte(tt.body)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %v, want %q", err, tt.want)
			}
		})
	}
	d := mustParseDocument(t, " \r\n\u00a0")
	if d.Format != FormatLinks || len(d.Profiles)+len(d.Skipped) != 0 {
		t.Fatalf("an empty body holds nothing: %+v", d)
	}
}
