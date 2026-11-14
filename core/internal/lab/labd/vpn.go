package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"PorosCore/internal/boxmain"
	// Xray's protocols and transports register themselves on import, as in the engine.
	_ "PorosCore/internal/distro/all"
	"PorosCore/internal/xray"
)

// The lab's VPN servers listen on every address of their namespace: vpn.lab is 203.0.113.10 and 3fff:0:203::10,
// 203.0.113.11 is the same servers reached by address. What they fetch leaves from 203.0.113.10, which the lab's
// internet tells from the user's own address.
const (
	portTrojan    = 8443
	portVLESSWS   = 8444
	portReality   = 8445
	portVMess     = 8446
	portSS        = 8447
	portHysteria2 = 8448
	portTUIC      = 8449
	portWireGuard = 51820
	portOpenVPN   = 1194
)

func runVPN(dir string) error {
	ca, err := loadCA(dir)
	if err != nil {
		return err
	}
	_, certPEM, keyPEM, err := ca.issue("vpn.lab", "203.0.113.10", "203.0.113.11", "3fff:0:203::10", "3fff:0:203::11")
	if err != nil {
		return err
	}
	caPEM, err := os.ReadFile(filepath.Join(dir, caCertFile))
	if err != nil {
		return err
	}
	password := randomText(16)
	uuid := randomUUID()
	ssKey := base64.StdEncoding.EncodeToString(randomBytes(16))
	wgServer, wgServerPublic := x25519(base64.StdEncoding)
	wgClient, wgClientPublic := x25519(base64.StdEncoding)
	realityPrivate, realityPublic := x25519(base64.RawURLEncoding)

	tlsServer := map[string]any{"enabled": true, "server_name": "vpn.lab", "certificate": string(certPEM), "key": string(keyPEM)}
	quicTLS := map[string]any{"enabled": true, "server_name": "vpn.lab", "certificate": string(certPEM), "key": string(keyPEM),
		"alpn": []string{"h3"}}
	users := []map[string]any{{"name": "lab", "password": password}}
	config := map[string]any{
		"log": map[string]any{"level": "warn"},
		"inbounds": []map[string]any{
			{"type": "trojan", "tag": "trojan", "listen": "::", "listen_port": portTrojan, "users": users, "tls": tlsServer},
			{"type": "vless", "tag": "vless-ws", "listen": "::", "listen_port": portVLESSWS,
				"users": []map[string]any{{"name": "lab", "uuid": uuid}}, "tls": tlsServer,
				"transport": map[string]any{"type": "ws", "path": "/ws"}},
			{"type": "vmess", "tag": "vmess-grpc", "listen": "::", "listen_port": portVMess,
				"users": []map[string]any{{"name": "lab", "uuid": uuid, "alterId": 0}}, "tls": tlsServer,
				"transport": map[string]any{"type": "grpc", "service_name": "lab"}},
			{"type": "shadowsocks", "tag": "ss", "listen": "::", "listen_port": portSS,
				"method": "2022-blake3-aes-128-gcm", "password": ssKey},
			{"type": "hysteria2", "tag": "hysteria2", "listen": "::", "listen_port": portHysteria2, "users": users, "tls": quicTLS},
			{"type": "tuic", "tag": "tuic", "listen": "::", "listen_port": portTUIC,
				"users":              []map[string]any{{"name": "lab", "uuid": uuid, "password": password}},
				"congestion_control": "bbr", "tls": quicTLS},
		},
		"endpoints": []map[string]any{
			{"type": "wireguard", "tag": "wireguard", "listen_port": portWireGuard,
				"address": []string{"10.66.0.1/24", "fd66::1/64"}, "private_key": wgServer,
				"peers": []map[string]any{{"public_key": wgClientPublic, "allowed_ips": []string{"10.66.0.2/32", "fd66::2/128"}}}},
			{"type": "openvpn-server", "tag": "openvpn", "listen": "::", "listen_port": portOpenVPN, "network": "tcp",
				"address": []string{"10.8.0.1/24"}, "users": []map[string]any{{"username": "lab", "password": password}},
				"tls": map[string]any{"certificate": string(certPEM), "key": string(keyPEM), "verify_client_certificate": "none"}},
		},
		"outbounds": []map[string]any{{"type": "direct", "tag": "direct"}},
		"route":     map[string]any{"final": "direct"},
	}
	content, err := json.Marshal(config)
	if err != nil {
		return err
	}
	box, cancel, err := boxmain.Create(content, nil)
	if err != nil {
		return fmt.Errorf("sing-box: %w", err)
	}
	defer func() {
		cancel()
		box.Close()
	}()

	// REALITY runs on Xray, as Poros runs it: the handshake borrows the lab's own HTTPS site.
	xrayConfig := fmt.Sprintf(`{"log":{"loglevel":"warning"},"inbounds":[{"listen":"::","port":%d,"protocol":"vless",`+
		`"settings":{"clients":[{"id":"%s"}],"decryption":"none"},"streamSettings":{"network":"tcp","security":"reality",`+
		`"realitySettings":{"target":"198.51.100.30:443","serverNames":["reality.lab"],"privateKey":"%s","shortIds":[""]}}}],`+
		// The lab's internet is all on reserved ranges, which freedom refuses unless told otherwise.
		`"outbounds":[{"protocol":"freedom","settings":{"finalRules":[{"action":"allow","ip":["0.0.0.0/0","::/0"]}]}}]}`,
		portReality, uuid, realityPrivate)
	instance, err := xray.CreateXrayInstance(xrayConfig)
	if err != nil {
		return fmt.Errorf("xray: %w", err)
	}
	if err = instance.Start(); err != nil {
		return fmt.Errorf("xray: %w", err)
	}
	defer instance.Close()

	name := func(text string) string { return "#" + url.PathEscape(text) }
	vmess, _ := json.Marshal(map[string]string{"v": "2", "ps": "Lab VMess gRPC", "add": "vpn.lab", "port": fmt.Sprint(portVMess),
		"id": uuid, "aid": "0", "scy": "auto", "net": "grpc", "type": "gun", "path": "lab", "tls": "tls", "sni": "vpn.lab"})
	openvpn, _ := json.Marshal(map[string]any{"type": "openvpn", "name": "Lab OpenVPN", "server": "vpn.lab",
		"server_port": portOpenVPN, "openvpn": map[string]any{"network": "tcp", "username": "lab", "password": password,
			"ca": string(caPEM)}})
	links := []string{
		fmt.Sprintf("trojan://%s@vpn.lab:%d?security=tls&sni=vpn.lab&type=tcp", password, portTrojan) + name("Lab Trojan"),
		fmt.Sprintf("trojan://%s@203.0.113.11:%d?security=tls&sni=vpn.lab&type=tcp", password, portTrojan) + name("Lab Trojan IP"),
		fmt.Sprintf("vless://%s@vpn.lab:%d?encryption=none&security=tls&sni=vpn.lab&type=ws&path=%%2Fws&host=vpn.lab", uuid,
			portVLESSWS) + name("Lab VLESS WS"),
		fmt.Sprintf("vless://%s@vpn.lab:%d?encryption=none&security=reality&sni=reality.lab&fp=chrome&pbk=%s&type=tcp", uuid,
			portReality, realityPublic) + name("Lab REALITY"),
		"vmess://" + base64.StdEncoding.EncodeToString(vmess),
		fmt.Sprintf("ss://%s@vpn.lab:%d", base64.RawURLEncoding.EncodeToString([]byte("2022-blake3-aes-128-gcm:"+ssKey)), portSS) +
			name("Lab Shadowsocks"),
		fmt.Sprintf("hysteria2://%s@vpn.lab:%d?sni=vpn.lab", password, portHysteria2) + name("Lab Hysteria2"),
		fmt.Sprintf("tuic://%s:%s@vpn.lab:%d?sni=vpn.lab&congestion_control=bbr&alpn=h3", uuid, password, portTUIC) + name("Lab TUIC"),
		fmt.Sprintf("wireguard://%s@vpn.lab:%d?publickey=%s&address=%s", url.QueryEscape(wgClient), portWireGuard,
			url.QueryEscape(wgServerPublic), url.QueryEscape("10.66.0.2/32,fd66::2/128")) + name("Lab WireGuard"),
		"poros://add/" + base64.RawURLEncoding.EncodeToString(openvpn),
	}
	sub := filepath.Join(dir, "sub.txt")
	if err = os.WriteFile(sub+".tmp", []byte(strings.Join(links, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	if err = os.Rename(sub+".tmp", sub); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "labd vpn: ready")
	waitForSignal()
	return nil
}

func randomBytes(size int) []byte {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return data
}

func randomText(size int) string { return hex.EncodeToString(randomBytes(size))[:size] }

func randomUUID() string {
	data := randomBytes(16)
	data[6] = data[6]&0x0f | 0x40
	data[8] = data[8]&0x3f | 0x80
	text := hex.EncodeToString(data)
	return text[:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:]
}

// x25519 makes a key pair for WireGuard (standard base64) or REALITY (URL base64 without padding).
func x25519(encoding *base64.Encoding) (private, public string) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return encoding.EncodeToString(key.Bytes()), encoding.EncodeToString(key.PublicKey().Bytes())
}
