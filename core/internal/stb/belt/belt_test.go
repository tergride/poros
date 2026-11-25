package belt

import (
	"bytes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"encoding/hex"
	"strings"
	"testing"
)

// The vectors come from the annexes of STB 34.101.31 and STB 34.101.47, as bee2 checks them: the data, keys and
// IVs are slices of the table H.

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func expect(t *testing.T, name string, got []byte, want string) {
	t.Helper()
	if w := unhex(t, want); !bytes.Equal(got, w) {
		t.Errorf("%s:\n got %X\nwant %X", name, got, w)
	}
}

var (
	encKey = H[128:160]
	decKey = H[160:192]
	encIV  = H[192:208]
	decIV  = H[208:224]
)

func TestTable(t *testing.T) {
	// The table rebuilds from the generator of the standard.
	var g [256]byte
	g[10], g[11] = 0, 0x8E
	for x := 12; x < 10+256; x++ {
		v := uint(g[(x-1)%256])
		for i := 0; i < 116; i++ {
			p := v & 0x63
			p ^= p >> 4
			p ^= p >> 2
			p ^= p >> 1
			v = v>>1 | (p&1)<<7
		}
		g[x%256] = byte(v)
	}
	if g != H {
		t.Fatal("H does not match its generator")
	}
}

func TestBlock(t *testing.T) {
	c, _ := NewCipher(encKey)
	out := make([]byte, 16)
	c.Encrypt(out, H[:16])
	expect(t, "A.1", out, "69CCA1C93557C9E3D66BC3E0FA88FA6E")
	c.Decrypt(out, out)
	expect(t, "A.1 back", out, hex.EncodeToString(H[:16]))
	d, _ := NewCipher(decKey)
	d.Decrypt(out, H[64:80])
	expect(t, "A.4", out, "0DC5300600CAB840B38448E5E993F421")
	for _, n := range []int{16, 24} {
		k, _ := NewCipher(H[128 : 128+n])
		k.Encrypt(out, H[:16])
		k.Decrypt(out, out)
		expect(t, "short key", out, hex.EncodeToString(H[:16]))
	}
	if _, err := NewCipher(make([]byte, 20)); err == nil {
		t.Fatal("a 20-byte key was taken")
	}
}

func TestWBL(t *testing.T) {
	out, _ := WBLEncrypt(encKey, H[:48])
	expect(t, "A.6-1", out, "49A38EE108D6C742E52B774F00A6EF98B106CBD13EA4FB0680323051BC04DF76E487B055C69BCF541176169F1DC9F6C8")
	out, _ = WBLEncrypt(encKey, H[:47])
	expect(t, "A.6-2", out, "F08EF22DCAA06C81FB12721974221CA7AB82C62856FCF2F9FCA006E019A28F16E5821A51F573594625DBAB8F6A5C94")
	out, _ = WBLDecrypt(decKey, H[64:112])
	expect(t, "A.7-1", out, "92632EE0C21AD9E09A39343E5C07DAA4889B03F2E6847EB152EC99F7A4D9F154B5EF68D8E4A39E567153DE13D72254EE")
	out, _ = WBLDecrypt(decKey, H[64:100])
	expect(t, "A.7-2", out, "DF3F882230BAAFFC92F05660321172310E3CB2182681EF43102E67175E177BD75E93E4E8")
	for n := 32; n <= 128; n++ {
		enc, _ := WBLEncrypt(encKey, H[:n])
		dec, _ := WBLDecrypt(encKey, enc)
		if !bytes.Equal(dec, H[:n]) {
			t.Fatalf("round trip of %d bytes", n)
		}
	}
}

func TestCBCAndCFB(t *testing.T) {
	// The data channel of OpenVPN pads to whole blocks, where belt-cbc and belt-cfb are the usual CBC and CFB.
	c, _ := NewCipher(encKey)
	out := make([]byte, 48)
	cipher.NewCBCEncrypter(c, encIV).CryptBlocks(out, H[:48])
	expect(t, "A.11-1", out, "10116EFAE6AD58EE14852E11DA1B8A745CF2480E8D03F1C19492E53ED3A70F60657C1EE8C0E0AE5B58388BF8A68E3309")
	d, _ := NewCipher(decKey)
	cipher.NewCBCDecrypter(d, decIV).CryptBlocks(out, H[64:112])
	expect(t, "A.12-1", out, "730894D6158E17CC1600185A8F411CAB0471FF85C83792398D8924EBD57D03DB95B97A9B7907E4B020960455E46176F8")
	//nolint:staticcheck // belt-cfb is what OpenVPN|BY offers.
	cipher.NewCFBEncrypter(c, encIV).XORKeyStream(out, H[:48])
	expect(t, "A.13", out, "C31E490A90EFA374626CC99E4B7B8540A6E48685464A5A06849C9CA769A1B0AE55C2CC5939303EC832DD2FE16C8E5A1B")
	//nolint:staticcheck // belt-cfb is what OpenVPN|BY offers.
	cipher.NewCFBDecrypter(d, decIV).XORKeyStream(out, H[64:112])
	expect(t, "A.14", out, "FA9D107A86F375EE65CD1DB881224BD016AFF814938ED39B3361ABB0BF0851B652244EB06842DD4C94AA4500774E40BB")
}

func TestCTR(t *testing.T) {
	s, _ := NewCTR(encKey, encIV)
	out := make([]byte, 48)
	s.XORKeyStream(out[:15], H[:15])
	s.XORKeyStream(out[15:22], H[15:22])
	s.XORKeyStream(out[22:], H[22:48])
	expect(t, "A.15", out, "52C9AF96FF50F64435FC43DEF56BD797D5B5B1FF79FB41257AB9CDF6E63E81F8F00341473EAE409833622DE05213773A")
	s, _ = NewCTR(decKey, decIV)
	out = out[:44]
	s.XORKeyStream(out, H[64:108])
	expect(t, "A.16", out, "DF181ED008A20F43DCBBB93650DAD34B389CDEE5826D40E2D4BD80F49A93F5D212F6333166456F169043CC5F")
}

func TestMAC(t *testing.T) {
	mac, _ := MAC(encKey, H[:13])
	expect(t, "A.17-1", mac[:], "7260DA60138F96C9")
	mac, _ = MAC(encKey, H[:48])
	expect(t, "A.17-2", mac[:], "2DAB59771B4B16D0")
}

func TestDWP(t *testing.T) {
	a, _ := NewDWP(encKey)
	sealed := a.Seal(nil, encIV, H[:16], H[16:48])
	expect(t, "A.19-1", sealed, "52C9AF96FF50F64435FC43DEF56BD797"+"3B2E0AEB2B91854B")
	b, _ := NewDWP(decKey)
	opened, err := b.Open(nil, decIV, append(append([]byte(nil), H[64:80]...), unhex(t, "6A2C2C94C4150DC0")...), H[80:112])
	if err != nil {
		t.Fatal(err)
	}
	expect(t, "A.20-1", opened, "DF181ED008A20F43DCBBB93650DAD34B")
	sealed[0] ^= 1
	if _, err := a.Open(nil, encIV, sealed, H[16:48]); err == nil {
		t.Fatal("a changed message opened")
	}
	for _, n := range []int{0, 1, 15, 17, 40} {
		for _, m := range []int{0, 5, 16, 33} {
			s := a.Seal(nil, encIV, H[:n], H[100:100+m])
			o, err := a.Open(nil, encIV, s, H[100:100+m])
			if err != nil || !bytes.Equal(o, H[:n]) {
				t.Fatalf("round trip %d/%d: %v", n, m, err)
			}
		}
	}
}

func TestKeyWrap(t *testing.T) {
	token, _ := KeyWrap(encKey, H[:32], H[32:48])
	expect(t, "A.21", token, "49A38EE108D6C742E52B774F00A6EF98B106CBD13EA4FB0680323051BC04DF76E487B055C69BCF541176169F1DC9F6C8")
	header := unhex(t, "B5EF68D8E4A39E567153DE13D72254EE")
	key, err := KeyUnwrap(decKey, H[64:112], header)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, "A.22", key, "92632EE0C21AD9E09A39343E5C07DAA4889B03F2E6847EB152EC99F7A4D9F154")
	if _, err := KeyUnwrap(decKey, H[64:112], nil); err != ErrBadToken {
		t.Fatalf("a wrong header gave %v", err)
	}
}

func TestHash(t *testing.T) {
	h := Hash(H[:13])
	expect(t, "A.23-1", h[:], "ABEF9725D4C5A83597A367D14494CC2542F20F659DDFECC961A3EC550CBA8C75")
	h = Hash(H[:32])
	expect(t, "A.23-2", h[:], "749E4C3653AECE5E48DB4761227742EB6DBE13F4A80F7BEFF1A9CF8D10EE7786")
	d := NewHash()
	d.Write(H[:11])
	d.Sum(nil)
	d.Write(H[11:48])
	expect(t, "A.23-3", d.Sum(nil), "9D02EE446FB6A29FE5C982D4B13AF9D3E90861BC4CEF27CF306BFB0B174A154A")
	for split := 0; split <= 48; split++ {
		d.Reset()
		d.Write(H[:split])
		d.Write(H[split:48])
		expect(t, "split", d.Sum(nil), "9D02EE446FB6A29FE5C982D4B13AF9D3E90861BC4CEF27CF306BFB0B174A154A")
	}
}

func TestHMACAndPBKDF2(t *testing.T) {
	for _, c := range []struct {
		keyLen int
		want   string
	}{
		{29, "D4828E6312B08BB83C9FA6535A4635549E411FD11C0D8289359A1130E930676B"},
		{32, "41FFE8645AEC0612E952D2CDF8DD508F3E4A1D9B53F6A1DB293B19FE76B1879F"},
		{42, "7D01B84D2315C332277B3653D7EC64707EBA7CDFF7FF70077B1DECBD68F2A144"},
	} {
		m := hmac.New(NewHash, H[128:128+c.keyLen])
		m.Write(H[192:224])
		expect(t, "Б.1", m.Sum(nil), c.want)
	}
	for _, c := range []struct {
		password string
		salt     []byte
		iter     int
		want     string
	}{
		{"B194BAC80A08F53B", H[192:200], 10000, "3D331BBBB1FBBB40E4BF22F6CB9A689EF13A77DC09ECF93291BFE42439A72E7D"},
		{"zed", unhex(t, "49FEFF8076CD9480"), 2048, "7249B4785FE68B1586D189A23E3842E48705C080A3248D8F0E8C3D63A93B2670"},
		{"zed", unhex(t, "C65017E4F108BCF0"), 10000, "E48329259BC1211DDAC2EF1DADFFC9932702A92F1DD66C14A9BA1D7300C8713C"},
	} {
		key, err := pbkdf2.Key(NewHash, c.password, c.salt, c.iter, 32)
		if err != nil {
			t.Fatal(err)
		}
		expect(t, "PBKDF2", key, c.want)
	}
	// STB 34.101.45, Е.5: the private key of Г.1 under the key of the password above.
	key, _ := pbkdf2.Key(NewHash, "B194BAC80A08F53B", H[192:200], 10000, 32)
	token, _ := KeyWrap(key, unhex(t, "1F66B5B84B7339674533F0329C74F21834281FED0732429E0C79235FC273E269"), nil)
	expect(t, "Е.5", token, "4EA289D5F718087DD8EDB305BA1CE8980E5EC3E0B56C8BF9D5C3E909CF4C14F07B8204E67841A165E924945CD07F37E7")
}
