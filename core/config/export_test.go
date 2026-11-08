package config

import (
	"encoding/json"
	"testing"
)

func TestExportPicksTheCoreAndKeepsNames(t *testing.T) {
	reality := mustParse(t, "vless://b831381d-6324-4d53-ad4f-8cda48b30811@de1.example.com:443?security=reality"+
		"&sni=www.example.com&fp=chrome&pbk=Z84J2IelR9ch3k8VtlVhhs5ycBUlXA7wHBWcBrjqnAw&sid=ab&type=tcp#Germany")
	trojan := mustParse(t, "trojan://secret@tr.example.com:443?security=tls&sni=tr.example.com&type=ws&path=%2Ftr")

	onXray, data, err := Export(reality, true)
	var xray map[string]any
	if err != nil || !onXray || json.Unmarshal(data, &xray) != nil {
		t.Fatalf("REALITY on Xray: %v, %s", err, data)
	}
	settings, _ := xray["settings"].(map[string]any)
	stream, _ := xray["streamSettings"].(map[string]any)
	if xray["protocol"] != "vless" || settings["address"] != "de1.example.com" || stream["sockopt"] != nil {
		t.Errorf("the Xray outbound keeps the name and binds no interface: %s", data)
	}

	onXray, data, err = Export(reality, false)
	var box map[string]any
	if err != nil || onXray || json.Unmarshal(data, &box) != nil || box["type"] != "vless" {
		t.Errorf("REALITY on sing-box: %v, %s", err, data)
	}
	onXray, data, err = Export(trojan, true)
	if err != nil || onXray || json.Unmarshal(data, &box) != nil || box["type"] != "trojan" || box["server"] != "tr.example.com" {
		t.Errorf("Trojan: %v, %s", err, data)
	}

	broken := trojan
	broken.Server = ""
	if _, _, err = Export(broken, true); err == nil {
		t.Error("an invalid server is exported")
	}

	links := sampleLinks(t)
	_, data, err = Export(mustParse(t, links["wireguard"]), true)
	var endpoint map[string]any
	if err != nil || json.Unmarshal(data, &endpoint) != nil || endpoint["type"] != "wireguard" || endpoint["peers"] == nil {
		t.Errorf("WireGuard is an endpoint: %v, %s", err, data)
	}
	_, data, err = Export(mustParse(t, links["ss over shadowtls"]), true)
	var pair struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err != nil || json.Unmarshal(data, &pair) != nil || len(pair.Outbounds) != 2 ||
		pair.Outbounds[0]["detour"] != pair.Outbounds[1]["tag"] || pair.Outbounds[1]["type"] != "shadowtls" {
		t.Errorf("Shadowsocks over ShadowTLS: %v, %s", err, data)
	}
}
