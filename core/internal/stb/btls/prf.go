package btls

import (
	"crypto/hmac"
	"encoding/binary"
	"errors"

	"PorosCore/internal/stb/belt"
)

const (
	masterSecretLength = 48
	finishedLength     = 12
	randomLength       = 32
)

const (
	labelMasterSecret         = "master secret"
	labelExtendedMasterSecret = "extended master secret"
	labelKeyExpansion         = "key expansion"
	labelClientFinished       = "client finished"
	labelServerFinished       = "server finished"
)

// pHash is P_hash of RFC 5246 over hmac-hbelt.
func pHash(result, secret, seed []byte) {
	h := hmac.New(belt.NewHash, secret)
	h.Write(seed)
	a := h.Sum(nil)
	for j := 0; j < len(result); {
		h.Reset()
		h.Write(a)
		h.Write(seed)
		j += copy(result[j:], h.Sum(nil))
		h.Reset()
		h.Write(a)
		a = h.Sum(a[:0])
	}
}

func prf(secret []byte, label string, seed []byte, length int) []byte {
	labelAndSeed := make([]byte, 0, len(label)+len(seed))
	labelAndSeed = append(append(labelAndSeed, label...), seed...)
	out := make([]byte, length)
	pHash(out, secret, labelAndSeed)
	return out
}

func masterSecret(preMaster, clientRandom, serverRandom []byte) []byte {
	seed := make([]byte, 0, 2*randomLength)
	seed = append(append(seed, clientRandom...), serverRandom...)
	return prf(preMaster, labelMasterSecret, seed, masterSecretLength)
}

// extendedMasterSecret is the master secret of RFC 7627 over the hash of the messages up to ClientKeyExchange.
func extendedMasterSecret(preMaster, transcript []byte) []byte {
	sum := belt.Hash(transcript)
	return prf(preMaster, labelExtendedMasterSecret, sum[:], masterSecretLength)
}

func finishedSum(master []byte, label string, transcript []byte) []byte {
	sum := belt.Hash(transcript)
	return prf(master, label, sum[:], finishedLength)
}

type trafficKeys struct {
	clientMAC, serverMAC []byte
	clientKey, serverKey []byte
	clientIV, serverIV   []byte
}

func keysFromMaster(suite *cipherSuite, master, clientRandom, serverRandom []byte) trafficKeys {
	macLen, keyLen, ivLen := suite.keyLengths()
	seed := make([]byte, 0, 2*randomLength)
	seed = append(append(seed, serverRandom...), clientRandom...)
	block := prf(master, labelKeyExpansion, seed, 2*(macLen+keyLen+ivLen))
	next := func(n int) []byte {
		out := block[:n:n]
		block = block[n:]
		return out
	}
	var k trafficKeys
	k.clientMAC, k.serverMAC = next(macLen), next(macLen)
	k.clientKey, k.serverKey = next(keyLen), next(keyLen)
	k.clientIV, k.serverIV = next(ivLen), next(ivLen)
	return k
}

var errReservedLabel = errors.New("btls: reserved exporter label")

// exportKeyingMaterial is the exporter of RFC 5705; a nil context and an empty one differ.
func exportKeyingMaterial(master, clientRandom, serverRandom []byte, label string, context []byte, length int) ([]byte, error) {
	switch label {
	case labelClientFinished, labelServerFinished, labelMasterSecret, labelKeyExpansion, labelExtendedMasterSecret:
		return nil, errReservedLabel
	}
	if length < 0 {
		return nil, errors.New("btls: negative length of keying material")
	}
	seed := make([]byte, 0, 2*randomLength+2+len(context))
	seed = append(append(seed, clientRandom...), serverRandom...)
	if context != nil {
		if len(context) >= 1<<16 {
			return nil, errors.New("btls: exporter context too long")
		}
		seed = binary.BigEndian.AppendUint16(seed, uint16(len(context)))
		seed = append(seed, context...)
	}
	return prf(master, label, seed, length), nil
}
