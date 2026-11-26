package profile

// Secret is a field of a profile that holds a secret, named by its path in the profile as the editor names its fields.
type Secret struct {
	Name  string
	Value *string
}

// Secrets gives the fields of a profile that hold secrets: passwords, keys, UUIDs, client certificates and the keys
// of an OpenVPN control channel. The certificate of a server's authority is no secret. The JSON of a custom outbound
// is one too, but it is no string field and its keeper takes it as a whole.
func (p *Profile) Secrets() []Secret {
	var list []Secret
	add := func(name string, value *string) { list = append(list, Secret{Name: name, Value: value}) }
	if o := p.VLESS; o != nil {
		add("vless.uuid", &o.UUID)
	}
	if o := p.VMess; o != nil {
		add("vmess.uuid", &o.UUID)
	}
	if o := p.Trojan; o != nil {
		add("trojan.password", &o.Password)
	}
	if o := p.Shadowsocks; o != nil {
		add("shadowsocks.password", &o.Password)
		add("shadowsocks.shadowtls.password", &o.ShadowTLS.Password)
	}
	if o := p.Hysteria2; o != nil {
		add("hysteria2.password", &o.Password)
		add("hysteria2.obfs_password", &o.ObfsPassword)
		add("hysteria2.realm.token", &o.Realm.Token)
	}
	if o := p.Hysteria; o != nil {
		add("hysteria.auth", &o.Auth)
		add("hysteria.obfs", &o.Obfs)
	}
	if o := p.TUIC; o != nil {
		add("tuic.uuid", &o.UUID)
		add("tuic.password", &o.Password)
	}
	if o := p.SOCKS; o != nil {
		add("socks.password", &o.Password)
	}
	if o := p.HTTP; o != nil {
		add("http.password", &o.Password)
	}
	if o := p.AnyTLS; o != nil {
		add("anytls.password", &o.Password)
	}
	if o := p.WireGuard; o != nil {
		add("wireguard.private_key", &o.PrivateKey)
		add("wireguard.pre_shared_key", &o.PreSharedKey)
	}
	if o := p.Naive; o != nil {
		add("naive.password", &o.Password)
	}
	if o := p.SSH; o != nil {
		add("ssh.password", &o.Password)
		add("ssh.private_key", &o.PrivateKey)
		add("ssh.private_key_passphrase", &o.PrivateKeyPassphrase)
	}
	if o := p.Snell; o != nil {
		add("snell.psk", &o.PSK)
		add("snell.userkey", &o.UserKey)
	}
	if o := p.OpenVPN; o != nil {
		add("openvpn.password", &o.Password)
		add("openvpn.cert", &o.Cert)
		add("openvpn.key", &o.Key)
		add("openvpn.key_password", &o.KeyPassword)
		add("openvpn.control_wrap.key", &o.ControlWrap.Key)
	}
	if o := p.OpenConnect; o != nil {
		add("openconnect.password", &o.Password)
		add("openconnect.cert", &o.Cert)
		add("openconnect.key", &o.Key)
		add("openconnect.key_password", &o.KeyPassword)
	}
	if o := p.MASQUE; o != nil {
		add("masque.password", &o.Password)
	}
	return list
}
