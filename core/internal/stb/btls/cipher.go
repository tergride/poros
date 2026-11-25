package btls

import (
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"errors"

	"PorosCore/internal/stb/belt"
)

type recordType uint8

const (
	recordTypeChangeCipherSpec recordType = 20
	recordTypeAlert            recordType = 21
	recordTypeHandshake        recordType = 22
	recordTypeApplicationData  recordType = 23
)

var errBadRecord = errors.New("btls: record authentication failed")

// recordCipher protects the records of one direction; seq is the number of the record since ChangeCipherSpec.
type recordCipher interface {
	seal(seq uint64, typ recordType, plaintext []byte) []byte
	open(seq uint64, typ recordType, payload []byte) ([]byte, error)
}

// additionalData is the header of a record as OpenSSL feeds it to its AEAD ciphers: seq ‖ type ‖ version ‖ length
// of the plaintext.
func additionalData(seq uint64, typ recordType, length int) [13]byte {
	var ad [13]byte
	binary.BigEndian.PutUint64(ad[:8], seq)
	ad[8] = byte(typ)
	binary.BigEndian.PutUint16(ad[9:], versionTLS12)
	binary.BigEndian.PutUint16(ad[11:], uint16(length))
	return ad
}

func newRecordCipher(suite *cipherSuite, macKey, key, iv []byte) (recordCipher, error) {
	if suite.dwp {
		aead, err := belt.NewDWP(key)
		if err != nil {
			return nil, err
		}
		c := &dwpCipher{aead: aead}
		copy(c.fixed[:], iv)
		return c, nil
	}
	c := &ctrMACCipher{key: append([]byte(nil), key...), macKey: append([]byte(nil), macKey...)}
	return c, nil
}

// dwpCipher is belt-dwp with the nonce fixed ‖ explicit, where fixed comes from the key block and the explicit half,
// the number of the record, leads the record.
type dwpCipher struct {
	aead  cipher.AEAD
	fixed [8]byte
}

const dwpExplicitLength = 8

func (c *dwpCipher) seal(seq uint64, typ recordType, plaintext []byte) []byte {
	var nonce [16]byte
	copy(nonce[:8], c.fixed[:])
	binary.BigEndian.PutUint64(nonce[8:], seq)
	ad := additionalData(seq, typ, len(plaintext))
	out := make([]byte, dwpExplicitLength, dwpExplicitLength+len(plaintext)+belt.DWPTagSize)
	copy(out, nonce[8:])
	return c.aead.Seal(out, nonce[:], plaintext, ad[:])
}

func (c *dwpCipher) open(seq uint64, typ recordType, payload []byte) ([]byte, error) {
	if len(payload) < dwpExplicitLength+belt.DWPTagSize {
		return nil, errBadRecord
	}
	var nonce [16]byte
	copy(nonce[:8], c.fixed[:])
	copy(nonce[8:], payload[:dwpExplicitLength])
	ad := additionalData(seq, typ, len(payload)-dwpExplicitLength-belt.DWPTagSize)
	plain, err := c.aead.Open(nil, nonce[:], payload[dwpExplicitLength:], ad[:])
	if err != nil {
		return nil, errBadRecord
	}
	return plain, nil
}

// ctrMACCipher is belt-mac over header ‖ plaintext, appended to the plaintext, then belt-ctr over both with the
// nonce seq ‖ 0, which is not sent.
type ctrMACCipher struct {
	key, macKey []byte
}

const macLength = 8

func (c *ctrMACCipher) stream(seq uint64) cipher.Stream {
	var iv [16]byte
	binary.BigEndian.PutUint64(iv[:8], seq)
	s, err := belt.NewCTR(c.key, iv[:])
	if err != nil {
		panic(err) // the key length is fixed by the suite
	}
	return s
}

func (c *ctrMACCipher) mac(seq uint64, typ recordType, plaintext []byte) [macLength]byte {
	ad := additionalData(seq, typ, len(plaintext))
	data := make([]byte, 0, len(ad)+len(plaintext))
	data = append(append(data, ad[:]...), plaintext...)
	tag, err := belt.MAC(c.macKey, data)
	if err != nil {
		panic(err)
	}
	return tag
}

func (c *ctrMACCipher) seal(seq uint64, typ recordType, plaintext []byte) []byte {
	tag := c.mac(seq, typ, plaintext)
	out := make([]byte, 0, len(plaintext)+macLength)
	out = append(append(out, plaintext...), tag[:]...)
	c.stream(seq).XORKeyStream(out, out)
	return out
}

func (c *ctrMACCipher) open(seq uint64, typ recordType, payload []byte) ([]byte, error) {
	if len(payload) < macLength {
		return nil, errBadRecord
	}
	plain := make([]byte, len(payload))
	c.stream(seq).XORKeyStream(plain, payload)
	n := len(plain) - macLength
	tag := c.mac(seq, typ, plain[:n])
	if subtle.ConstantTimeCompare(tag[:], plain[n:]) != 1 {
		return nil, errBadRecord
	}
	return plain[:n], nil
}
