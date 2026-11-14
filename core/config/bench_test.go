package config

import (
	"net/netip"
	"testing"

	"PorosCore/profile"
)

// Building a connection: a server on sing-box with the TUN, and REALITY on Xray behind the bridge.
func BenchmarkBuild(b *testing.B) {
	for name, link := range map[string]string{
		"trojan": "trojan://secret@t.example.com:443?security=tls&sni=t.example.com&type=ws&path=%2Fws#trojan",
		"reality": "vless://" + testUUID + "@r.example.com:443?encryption=none&flow=xtls-rprx-vision&security=reality" +
			"&sni=www.microsoft.com&fp=chrome&pbk=" + testPBK + "&sid=6ba85179e30d4fc2&type=tcp#reality",
	} {
		server, err := profile.ParseLink(link)
		if err != nil {
			b.Fatal(err)
		}
		in := Input{Server: server, Preset: allPreset, MixedPort: 2090, Tun: true, RealityOnXray: true,
			Bridge: testBridge, Interface: "Ethernet", Resolved: map[string]netip.Addr{}}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				configs, err := Build(in)
				if err != nil {
					b.Fatal(err)
				}
				if _, err = configs.SingBoxJSON(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
