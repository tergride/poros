package warp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A registration against a stand-in of Cloudflare's API: WireGuard takes the keys of the first answer, MASQUE
// enrolls a key of its own and takes the server's key from the second.
func TestRegister(t *testing.T) {
	server, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serverDER, _ := x509.MarshalPKIXPublicKey(&server.PublicKey)
	serverPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: serverDER}))
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var answer device
		answer.ID, answer.Token, answer.Account.License = "id", "token", "license"
		answer.Config.ClientID = "DFXJ"
		answer.Config.Interface.Addresses.V4, answer.Config.Interface.Addresses.V6 = "172.16.0.2", "2606:4700:110:8a2c::2"
		answer.Config.Peers = make([]struct {
			PublicKey string `json:"public_key"`
			Endpoint  struct {
				Host string `json:"host"`
				V4   string `json:"v4"`
				V6   string `json:"v6"`
			} `json:"endpoint"`
		}, 1)
		answer.Config.Peers[0].PublicKey = "bmQzcGxhY2Vob2xkZXJwbGFjZWhvbGRlcnBsYWNlaG8="
		answer.Config.Peers[0].Endpoint.Host = "engage.cloudflareclient.com:2408"
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reg"):
		case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/reg/id") && r.Header.Get("Authorization") == "Bearer token":
			answer.Config.Peers[0].PublicKey = serverPEM
		default:
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	defer api.Close()
	host := strings.TrimPrefix(api.URL, "https://")

	wg, err := Register(t.Context(), TunnelWireGuard, api.Client(), []string{host})
	if err != nil {
		t.Fatal(err)
	}
	if wg.Endpoint != "engage.cloudflareclient.com:2408" || len(wg.Reserved) != 3 || wg.PrivateKey == "" || wg.IPv4 != "172.16.0.2" {
		t.Errorf("WireGuard: %+v", wg)
	}
	masque, err := Register(t.Context(), TunnelMASQUE, api.Client(), []string{host})
	if err != nil {
		t.Fatal(err)
	}
	if masque.Endpoint != masqueEndpoint || masque.PeerPublicKey == "" || masque.PrivateKey == "" {
		t.Errorf("MASQUE: %+v", masque)
	}
	if _, err := Register(t.Context(), "openvpn", api.Client(), nil); err == nil {
		t.Error("an unknown tunnel must be refused")
	}
}
