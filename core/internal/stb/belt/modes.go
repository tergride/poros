package belt

import (
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"errors"
)

// ctr is belt-ctr. Its counter starts as belt-block(IV) and grows by one, little-endian, before each block; this
// is not the counter of NIST CTR, so crypto/cipher.NewCTR does not fit.
type ctr struct {
	key      [8]uint32
	counter  [4]uint32
	gamma    [BlockSize]byte
	reserved int
}

// NewCTR returns belt-ctr with a key of 16, 24 or 32 bytes and an IV of 16 bytes.
func NewCTR(key, iv []byte) (cipher.Stream, error) {
	c, err := newCTR(key, iv)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func newCTR(key, iv []byte) (*ctr, error) {
	k, err := expandKey(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != BlockSize {
		return nil, errors.New("belt: invalid IV size")
	}
	c := &ctr{key: k, counter: loadWords(iv)}
	encryptWords(&c.counter, &c.key)
	return c, nil
}

func (c *ctr) XORKeyStream(dst, src []byte) {
	if len(dst) < len(src) {
		panic("belt: output smaller than input")
	}
	for len(src) > 0 {
		if c.reserved == 0 {
			c.counter[0]++
			if c.counter[0] == 0 {
				c.counter[1]++
				if c.counter[1] == 0 {
					c.counter[2]++
					if c.counter[2] == 0 {
						c.counter[3]++
					}
				}
			}
			block := c.counter
			encryptWords(&block, &c.key)
			storeWords(c.gamma[:], block)
			c.reserved = BlockSize
		}
		n := subtle.XORBytes(dst, src, c.gamma[BlockSize-c.reserved:])
		c.reserved -= n
		dst, src = dst[n:], src[n:]
	}
}

// MAC returns belt-mac of data, 8 bytes, under a key of 16, 24 or 32 bytes.
func MAC(key, data []byte) ([8]byte, error) {
	var mac [8]byte
	k, err := expandKey(key)
	if err != nil {
		return mac, err
	}
	var s, r [4]uint32
	encryptWords(&r, &k)
	// Every block but the last goes through belt-block; the last, full or padded, is mixed with r first.
	for len(data) > BlockSize {
		x := loadWords(data)
		for i := range s {
			s[i] ^= x[i]
		}
		encryptWords(&s, &k)
		data = data[BlockSize:]
	}
	var last [BlockSize]byte
	copy(last[:], data)
	if len(data) == BlockSize {
		x := loadWords(last[:])
		s[0] ^= x[0] ^ r[1]
		s[1] ^= x[1] ^ r[2]
		s[2] ^= x[2] ^ r[3]
		s[3] ^= x[3] ^ r[0] ^ r[1]
	} else {
		last[len(data)] = 0x80
		x := loadWords(last[:])
		s[0] ^= x[0] ^ r[0] ^ r[3]
		s[1] ^= x[1] ^ r[0]
		s[2] ^= x[2] ^ r[1]
		s[3] ^= x[3] ^ r[2]
	}
	encryptWords(&s, &k)
	binary.LittleEndian.PutUint32(mac[0:], s[0])
	binary.LittleEndian.PutUint32(mac[4:], s[1])
	return mac, nil
}

func xorRound(block []byte, round uint64) {
	var r [8]byte
	binary.LittleEndian.PutUint64(r[:], round)
	subtle.XORBytes(block[:8], block[:8], r[:])
}

// wblEncrypt is belt-wblock encryption in place of a buffer of at least 32 bytes.
func wblEncrypt(buf []byte, k *[8]uint32) {
	count := len(buf)
	n := uint64((count + 15) / 16)
	var block [BlockSize]byte
	for round := uint64(1); round <= 2*n; round++ {
		// block = r1 + ... + r(n-1)
		copy(block[:], buf[:BlockSize])
		for i := BlockSize; i+BlockSize < count; i += BlockSize {
			subtle.XORBytes(block[:], block[:], buf[i:i+BlockSize])
		}
		copy(buf, buf[BlockSize:])
		copy(buf[count-BlockSize:], block[:])
		x := loadWords(block[:])
		encryptWords(&x, k)
		storeWords(block[:], x)
		xorRound(block[:], round)
		// The last block before the shift sits 16 bytes lower after it.
		subtle.XORBytes(buf[count-2*BlockSize:count-BlockSize], buf[count-2*BlockSize:count-BlockSize], block[:])
	}
}

// wblDecrypt is belt-wblock decryption in place of a buffer of at least 32 bytes.
func wblDecrypt(buf []byte, k *[8]uint32) {
	count := len(buf)
	n := uint64((count + 15) / 16)
	var block [BlockSize]byte
	for round := 2 * n; round >= 1; round-- {
		copy(block[:], buf[count-BlockSize:])
		copy(buf[BlockSize:], buf[:count-BlockSize])
		copy(buf[:BlockSize], block[:])
		x := loadWords(block[:])
		encryptWords(&x, k)
		storeWords(block[:], x)
		xorRound(block[:], round)
		subtle.XORBytes(buf[count-BlockSize:], buf[count-BlockSize:], block[:])
		for i := BlockSize; i+BlockSize < count; i += BlockSize {
			subtle.XORBytes(buf[:BlockSize], buf[:BlockSize], buf[i:i+BlockSize])
		}
	}
}

// WBLEncrypt returns belt-wblock encryption of data, at least 32 bytes, under a key of 16, 24 or 32 bytes.
func WBLEncrypt(key, data []byte) ([]byte, error) {
	k, err := expandKey(key)
	if err != nil {
		return nil, err
	}
	if len(data) < 2*BlockSize {
		return nil, errShortInput
	}
	out := append([]byte(nil), data...)
	wblEncrypt(out, &k)
	return out, nil
}

// WBLDecrypt reverses WBLEncrypt.
func WBLDecrypt(key, data []byte) ([]byte, error) {
	k, err := expandKey(key)
	if err != nil {
		return nil, err
	}
	if len(data) < 2*BlockSize {
		return nil, errShortInput
	}
	out := append([]byte(nil), data...)
	wblDecrypt(out, &k)
	return out, nil
}

// ErrBadToken reports a key token whose header does not match: a wrong key or damaged data.
var ErrBadToken = errors.New("belt: key token does not check out")

// KeyWrap returns belt-kwp protection of key, at least 16 bytes, with a header of 16 bytes or none for zeros.
func KeyWrap(kek, key, header []byte) ([]byte, error) {
	k, err := expandKey(kek)
	if err != nil {
		return nil, err
	}
	if len(key) < BlockSize {
		return nil, errShortInput
	}
	if header != nil && len(header) != BlockSize {
		return nil, errors.New("belt: invalid header size")
	}
	out := make([]byte, len(key)+BlockSize)
	copy(out, key)
	copy(out[len(key):], header)
	wblEncrypt(out, &k)
	return out, nil
}

// KeyUnwrap reverses KeyWrap and checks the header, nil standing for zeros.
func KeyUnwrap(kek, token, header []byte) ([]byte, error) {
	k, err := expandKey(kek)
	if err != nil {
		return nil, err
	}
	if len(token) < 2*BlockSize {
		return nil, ErrBadToken
	}
	if header == nil {
		header = make([]byte, BlockSize)
	} else if len(header) != BlockSize {
		return nil, errors.New("belt: invalid header size")
	}
	out := append([]byte(nil), token...)
	wblDecrypt(out, &k)
	if subtle.ConstantTimeCompare(out[len(out)-BlockSize:], header) != 1 {
		clear(out)
		return nil, ErrBadToken
	}
	return out[:len(out)-BlockSize], nil
}
