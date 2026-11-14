package subfetch

import (
	"encoding/base64"
	"net/http"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// hangLimit bounds one input. The fuzzing engine has no time limit of its own.
const hangLimit = 10 * time.Second

// watchHang crashes the process when an input runs past hangLimit, which keeps the input as a crasher.
func watchHang() (stop func()) {
	timer := time.AfterFunc(hangLimit, func() {
		debug.SetTraceback("all")
		panic("the input ran longer than " + hangLimit.String())
	})
	return func() { timer.Stop() }
}

// The provider writes the headers and the leading "#key: value" lines of the body: whatever they hold, reading them
// gives an Info within its bounds.
func FuzzProviderInfo(f *testing.F) {
	b64 := func(text string) string { return base64.StdEncoding.EncodeToString([]byte(text)) }
	f.Add("upload=100; download=2000; total=107374182400; expire=1765497600000", "12", "base64:"+b64("Основная подписка"),
		"https://example.com", "https://t.me/support", "", []byte("vless://id@de1.example.com:443#DE\n"))
	f.Add("", `"24"`, "From the header", "", "https://a.example\r\nX-Evil: 1", "base64:!!!", []byte("#profile-title: Work\n"+
		"// profile-update-interval: 2d\n#subscription-userinfo: upload=1; download=2; total=3; expire=4\n#announce: base64:"+
		b64("Maintenance tonight")+"\nvless://id@a.example:443\n#profile-web-page-url: https://ignored.example\n"))
	f.Add("UPLOAD = 99999999999999999999; Total=0", "99999999d", strings.Repeat("x", maxHeaderValue), "", "", "",
		[]byte(b64("#profile-title: Encoded\n#support-url: https://s.example\nvless://id@a.example:443\n")))
	f.Add("expire=1", "7200s", "", "", "", "base64:", []byte("//notice: base64:"+b64("hello")+"\n#subscription-userinfo: nothing\n"))
	f.Add("", "-5", "", "", "", "", []byte(b64("#profile-title: Cut")[:10]+"\n"+b64("#profile-title: Cut")[10:]))
	f.Fuzz(func(t *testing.T, userInfo, interval, title, webPage, support, announce string, body []byte) {
		defer watchHang()()
		h := http.Header{}
		for name, value := range map[string]string{
			"Subscription-Userinfo": userInfo, "Profile-Update-Interval": interval, "Profile-Title": title,
			"Profile-Web-Page-Url": webPage, "Support-Url": support, "Announce": announce,
		} {
			if value != "" {
				h[name] = []string{value}
			}
		}
		info := infoFromHeaders(h)
		for name, value := range map[string]string{"web page": info.WebPageURL, "support": info.SupportURL} {
			if len(value) >= maxHeaderValue || strings.ContainsAny(value, "\r\n") {
				t.Fatalf("the %s address %q of the headers was kept", name, value)
			}
		}
		fillFromBody(&info, body)
		if d := info.UpdateInterval; d != 0 && (d < time.Hour || d > maxUpdateInterval || d%time.Hour != 0) {
			t.Fatalf("the update interval %v", d)
		}
		if info.Upload < 0 || info.Download < 0 || info.Total < 0 || info.Expire < 0 || info.Total != 0 && !info.HasQuota {
			t.Fatalf("the traffic %+v", info)
		}
	})
}
