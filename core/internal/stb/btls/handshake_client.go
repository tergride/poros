package btls

import (
	"bytes"
	"crypto/subtle"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"slices"

	"PorosCore/internal/stb/bign"
	"PorosCore/internal/stb/stbkey"
)

type clientHandshake struct {
	c          *Conn
	transcript bytes.Buffer
	hello      *clientHello
	suite      *cipherSuite
	serverKey  *stbkey.PublicKey
	ephemeral  []byte
	request    *certificateRequest
}

func (c *Conn) clientHandshake() error {
	cfg := c.config
	suites := cfg.cipherSuites()
	if len(suites) == 0 {
		return errors.New("btls: no BTLS cipher suite is configured")
	}
	cert, err := cfg.certificate()
	if err != nil {
		return err
	}
	if err := c.setBuffering(true); err != nil {
		return err
	}
	hs := &clientHandshake{c: c}
	var curve *bign.Curve
	if cert != nil {
		curve = cert.PrivateKey.Curve
	}
	hs.hello = &clientHello{
		version:                      versionTLS12,
		random:                       make([]byte, randomLength),
		compressionMethods:           []uint8{compressionNone},
		signatureAlgorithms:          signatureAlgorithmsFor(curve),
		secureRenegotiationSupported: true,
		extendedMasterSecret:         true,
	}
	if _, err := io.ReadFull(cfg.rand(), hs.hello.random); err != nil {
		return fmt.Errorf("btls: short read from Rand: %w", err)
	}
	for _, s := range suites {
		hs.hello.cipherSuites = append(hs.hello.cipherSuites, s.id)
	}
	if err := c.writeHandshake(hs.hello.marshal(), &hs.transcript); err != nil {
		return err
	}
	if err := c.flushOut(); err != nil {
		return err
	}
	if err := hs.readServerHello(); err != nil {
		return err
	}
	if err := hs.readServerFlight(); err != nil {
		return err
	}
	master, err := hs.sendClientFlight(cert)
	if err != nil {
		return err
	}
	return hs.readServerFinished(master)
}

func (hs *clientHandshake) readServerHello() error {
	c := hs.c
	msg, err := c.readHandshake()
	if err != nil {
		return err
	}
	var hello serverHello
	if !hello.unmarshal(msg) {
		return c.badMessage(msg, typeServerHello)
	}
	hs.transcript.Write(msg)
	if hello.version != versionTLS12 {
		return c.fail(alertProtocolVersion, fmt.Errorf("btls: the server chose the version %x, only TLS 1.2 is supported", hello.version))
	}
	c.vers = versionTLS12
	if slices.Contains(hs.hello.cipherSuites, hello.cipherSuite) {
		hs.suite = cipherSuiteByID(hello.cipherSuite)
	}
	if hs.suite == nil {
		return c.fail(alertIllegalParameter, fmt.Errorf("btls: the server chose the cipher suite %04x it was not offered", hello.cipherSuite))
	}
	if hello.compressionMethod != compressionNone {
		return c.fail(alertIllegalParameter, errors.New("btls: the server chose compression"))
	}
	if hello.unknownExtension {
		return c.fail(alertUnsupportedExtension, errors.New("btls: the server sent an extension it was not offered"))
	}
	if hello.secureRenegotiationSupported && len(hello.secureRenegotiation) != 0 {
		return c.fail(alertHandshakeFailure, errors.New("btls: initial handshake had non-empty renegotiation extension"))
	}
	c.suite = hs.suite
	c.clientRandom, c.serverRandom = hs.hello.random, hello.random
	c.extendedMasterSecret = hello.extendedMasterSecret
	return nil
}

// readServerFlight reads Certificate, ServerKeyExchange of DHE, an optional CertificateRequest and ServerHelloDone.
func (hs *clientHandshake) readServerFlight() error {
	c := hs.c
	msg, err := c.readHandshake()
	if err != nil {
		return err
	}
	var certs certificateMsg
	if !certs.unmarshal(msg) {
		return c.badMessage(msg, typeCertificate)
	}
	hs.transcript.Write(msg)
	if err := hs.verifyServerCertificate(certs.certificates); err != nil {
		return err
	}
	if msg, err = c.readHandshake(); err != nil {
		return err
	}
	if hs.suite.ephemeral {
		var ske serverKeyExchange
		if !ske.unmarshal(msg) {
			return c.badMessage(msg, typeServerKeyExchange)
		}
		hs.transcript.Write(msg)
		if err := hs.verifyKeyExchange(&ske); err != nil {
			return err
		}
		if msg, err = c.readHandshake(); err != nil {
			return err
		}
	}
	if len(msg) > 0 && msg[0] == typeCertificateRequest {
		hs.request = new(certificateRequest)
		if !hs.request.unmarshal(msg) {
			return c.badMessage(msg, typeCertificateRequest)
		}
		hs.transcript.Write(msg)
		if msg, err = c.readHandshake(); err != nil {
			return err
		}
	}
	if !isEmptyMessage(msg, typeServerHelloDone) {
		return c.badMessage(msg, typeServerHelloDone)
	}
	hs.transcript.Write(msg)
	return nil
}

func (hs *clientHandshake) verifyServerCertificate(raw [][]byte) error {
	c := hs.c
	cfg := c.config
	if len(raw) == 0 {
		return c.fail(alertBadCertificate, errors.New("btls: the server sent no certificate"))
	}
	certs := make([]*x509.Certificate, len(raw))
	for i, der := range raw {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return c.fail(alertBadCertificate, fmt.Errorf("btls: failed to parse certificate from server: %w", err))
		}
		certs[i] = cert
	}
	var chains [][]*x509.Certificate
	if !cfg.InsecureSkipVerify {
		chain, err := VerifyChain(certs, cfg.RootCAs, cfg.now())
		if err != nil {
			return c.fail(certificateAlert(err), err)
		}
		chains = [][]*x509.Certificate{chain}
	}
	if cfg.VerifyPeerCertificate != nil {
		if err := cfg.VerifyPeerCertificate(raw, chains); err != nil {
			return c.fail(alertBadCertificate, err)
		}
	}
	key, err := stbkey.ParsePublicKeyInfo(certs[0].RawSubjectPublicKeyInfo)
	if err != nil {
		return c.fail(alertUnsupportedCertificate, fmt.Errorf("btls: the certificate of the server has no bign key: %w", err))
	}
	c.peerCertificates, c.verifiedChains = certs, chains
	hs.serverKey = key
	return nil
}

func certificateAlert(err error) alert {
	if errors.Is(err, ErrUnknownAuthority) {
		return alertUnknownCA
	}
	return alertBadCertificate
}

// verifyKeyExchange checks the ephemeral key of the server, which lies on the curve of its certificate.
func (hs *clientHandshake) verifyKeyExchange(ske *serverKeyExchange) error {
	c := hs.c
	curve := hs.serverKey.Curve
	if len(ske.key) != 2*curve.Size() || !curve.ValidPublicKey(ske.key) {
		return c.fail(alertIllegalParameter, errors.New("btls: the ephemeral key of the server is not a point of its curve"))
	}
	if signatureCurve(ske.signatureAlgorithm) != curve || !slices.Contains(hs.hello.signatureAlgorithms, ske.signatureAlgorithm) {
		return c.fail(alertIllegalParameter, fmt.Errorf("btls: the server signed with the algorithm %04x that does not fit its key", ske.signatureAlgorithm))
	}
	digest := signDigest(curve, c.clientRandom, c.serverRandom, ske.params)
	if !curve.Verify(hs.serverKey.Point, hashOIDDER[curve], digest, ske.signature) {
		return c.fail(alertDecryptError, errors.New("btls: invalid signature of the server key exchange"))
	}
	hs.ephemeral = ske.key
	return nil
}

func (hs *clientHandshake) keyExchange() (preMaster, data []byte, err error) {
	curve := hs.serverKey.Curve
	random := hs.c.config.rand()
	if hs.suite.ephemeral {
		priv, pub, err := curve.GenerateKey(random)
		if err != nil {
			return nil, nil, err
		}
		defer clear(priv)
		preMaster, err = curve.DH(priv, hs.ephemeral, 2*curve.Size())
		return preMaster, pub, err
	}
	preMaster = make([]byte, masterSecretLength)
	if _, err := io.ReadFull(random, preMaster); err != nil {
		return nil, nil, err
	}
	token, err := curve.KeyWrap(hs.serverKey.Point, preMaster, nil, random)
	return preMaster, token, err
}

// sendClientFlight sends Certificate when asked, ClientKeyExchange, CertificateVerify for a certificate,
// ChangeCipherSpec and Finished. It returns the master secret.
func (hs *clientHandshake) sendClientFlight(cert *Certificate) ([]byte, error) {
	c := hs.c
	cfg := c.config
	var signer *Certificate
	var signature uint16
	if hs.request != nil {
		if cert != nil {
			if alg, ok := signatureFor(cert.PrivateKey.Curve, hs.request.signatureAlgorithms); ok {
				signer, signature = cert, alg
			}
		}
		msg := new(certificateMsg)
		if signer != nil {
			msg.certificates = signer.Chain
		}
		if err := c.writeHandshake(msg.marshal(), &hs.transcript); err != nil {
			return nil, err
		}
	}
	preMaster, data, err := hs.keyExchange()
	if err != nil {
		return nil, c.fail(alertInternalError, fmt.Errorf("btls: key exchange: %w", err))
	}
	defer clear(preMaster)
	if err := c.writeHandshake(clientKeyExchangeMsg(data), &hs.transcript); err != nil {
		return nil, err
	}
	var master []byte
	if c.extendedMasterSecret {
		master = extendedMasterSecret(preMaster, hs.transcript.Bytes())
	} else {
		master = masterSecret(preMaster, c.clientRandom, c.serverRandom)
	}
	if signer != nil {
		key := signer.PrivateKey
		digest := signDigest(key.Curve, hs.transcript.Bytes())
		sig, err := key.Curve.Sign(key.D, hashOIDDER[key.Curve], digest, cfg.rand())
		if err != nil {
			return nil, c.fail(alertInternalError, fmt.Errorf("btls: failed to sign handshake: %w", err))
		}
		verify := &certificateVerify{signatureAlgorithm: signature, signature: sig}
		if err := c.writeHandshake(verify.marshal(), &hs.transcript); err != nil {
			return nil, err
		}
	}
	keys := keysFromMaster(hs.suite, master, c.clientRandom, c.serverRandom)
	clientCipher, err := newRecordCipher(hs.suite, keys.clientMAC, keys.clientKey, keys.clientIV)
	if err != nil {
		return nil, c.fail(alertInternalError, err)
	}
	serverCipher, err := newRecordCipher(hs.suite, keys.serverMAC, keys.serverKey, keys.serverIV)
	if err != nil {
		return nil, c.fail(alertInternalError, err)
	}
	c.in.next = serverCipher
	if err := c.writeChangeCipherSpec(clientCipher); err != nil {
		return nil, err
	}
	finished := finishedSum(master, labelClientFinished, hs.transcript.Bytes())
	if err := c.writeHandshake(finishedMsg(finished), &hs.transcript); err != nil {
		return nil, err
	}
	return master, c.flushOut()
}

func (hs *clientHandshake) readServerFinished(master []byte) error {
	c := hs.c
	if err := c.readChangeCipherSpec(); err != nil {
		return err
	}
	msg, err := c.readHandshake()
	if err != nil {
		return err
	}
	verify, ok := parseFinished(msg)
	if !ok {
		return c.badMessage(msg, typeFinished)
	}
	expected := finishedSum(master, labelServerFinished, hs.transcript.Bytes())
	if subtle.ConstantTimeCompare(expected, verify) != 1 {
		return c.fail(alertDecryptError, errors.New("btls: the Finished of the server does not check out"))
	}
	c.master = master
	return nil
}
