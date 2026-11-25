package btls

import (
	"bytes"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"slices"

	"PorosCore/internal/stb/stbkey"
)

type serverHandshake struct {
	c          *Conn
	transcript bytes.Buffer
	cert       *Certificate
	hello      clientHello
	suite      *cipherSuite
	signature  uint16
	ephemeral  []byte
	clientKey  *stbkey.PublicKey
}

func (c *Conn) serverHandshake() error {
	cert, err := c.config.certificate()
	if err != nil {
		return err
	}
	if cert == nil {
		return errors.New("btls: the server has no certificate")
	}
	if err := c.setBuffering(true); err != nil {
		return err
	}
	hs := &serverHandshake{c: c, cert: cert}
	defer func() { clear(hs.ephemeral) }()
	if err := hs.readClientHello(); err != nil {
		return err
	}
	if err := hs.sendServerFlight(); err != nil {
		return err
	}
	master, err := hs.readClientFlight()
	if err != nil {
		return err
	}
	return hs.finish(master)
}

func (hs *serverHandshake) readClientHello() error {
	c := hs.c
	msg, err := c.readHandshake()
	if err != nil {
		return err
	}
	if !hs.hello.unmarshal(msg) {
		return c.badMessage(msg, typeClientHello)
	}
	hs.transcript.Write(msg)
	if hs.hello.version < versionTLS12 {
		return c.fail(alertProtocolVersion, fmt.Errorf("btls: the client offers only the version %x, TLS 1.2 is required", hs.hello.version))
	}
	c.vers = versionTLS12
	if !slices.Contains(hs.hello.compressionMethods, compressionNone) {
		return c.fail(alertIllegalParameter, errors.New("btls: the client does not support uncompressed connections"))
	}
	if hs.hello.secureRenegotiationSupported && len(hs.hello.secureRenegotiation) != 0 {
		return c.fail(alertHandshakeFailure, errors.New("btls: initial handshake had non-empty renegotiation extension"))
	}
	curve := hs.cert.PrivateKey.Curve
	for _, s := range c.config.cipherSuites() {
		if !slices.Contains(hs.hello.cipherSuites, s.id) {
			continue
		}
		if s.ephemeral {
			alg, ok := signatureFor(curve, hs.hello.signatureAlgorithms)
			if !ok {
				continue
			}
			hs.signature = alg
		}
		hs.suite = s
		break
	}
	if hs.suite == nil {
		return c.fail(alertHandshakeFailure, errors.New("btls: no cipher suite in common with the client"))
	}
	return nil
}

// sendServerFlight sends ServerHello, Certificate, ServerKeyExchange of DHE, CertificateRequest when the policy asks
// for certificates, and ServerHelloDone.
func (hs *serverHandshake) sendServerFlight() error {
	c := hs.c
	cfg := c.config
	random := make([]byte, randomLength)
	if _, err := io.ReadFull(cfg.rand(), random); err != nil {
		return c.fail(alertInternalError, err)
	}
	hello := &serverHello{
		version:                      versionTLS12,
		random:                       random,
		cipherSuite:                  hs.suite.id,
		compressionMethod:            compressionNone,
		secureRenegotiationSupported: hs.hello.secureRenegotiationSupported,
		extendedMasterSecret:         hs.hello.extendedMasterSecret,
	}
	c.suite = hs.suite
	c.clientRandom, c.serverRandom = hs.hello.random, random
	c.extendedMasterSecret = hello.extendedMasterSecret
	if err := c.writeHandshake(hello.marshal(), &hs.transcript); err != nil {
		return err
	}
	certs := &certificateMsg{certificates: hs.cert.Chain}
	if err := c.writeHandshake(certs.marshal(), &hs.transcript); err != nil {
		return err
	}
	if hs.suite.ephemeral {
		key := hs.cert.PrivateKey
		priv, pub, err := key.Curve.GenerateKey(cfg.rand())
		if err != nil {
			return c.fail(alertInternalError, err)
		}
		hs.ephemeral = priv
		params := dhParams(pub)
		digest := signDigest(key.Curve, c.clientRandom, c.serverRandom, params)
		sig, err := key.Curve.Sign(key.D, hashOIDDER[key.Curve], digest, cfg.rand())
		if err != nil {
			return c.fail(alertInternalError, err)
		}
		ske := &serverKeyExchange{params: params, signatureAlgorithm: hs.signature, signature: sig}
		if err := c.writeHandshake(ske.marshal(), &hs.transcript); err != nil {
			return err
		}
	}
	if cfg.ClientAuth >= tls.RequestClientCert {
		request := &certificateRequest{
			certificateTypes:    []uint8{certificateTypeBignSign},
			signatureAlgorithms: supportedSignatureAlgorithms,
		}
		if err := c.writeHandshake(request.marshal(), &hs.transcript); err != nil {
			return err
		}
	}
	if err := c.writeHandshake(handshakeMessage(typeServerHelloDone, nil), &hs.transcript); err != nil {
		return err
	}
	return c.flushOut()
}

// readClientFlight reads Certificate when asked, ClientKeyExchange, CertificateVerify for a certificate,
// ChangeCipherSpec and Finished. It returns the master secret.
func (hs *serverHandshake) readClientFlight() ([]byte, error) {
	c := hs.c
	cfg := c.config
	msg, err := c.readHandshake()
	if err != nil {
		return nil, err
	}
	if cfg.ClientAuth >= tls.RequestClientCert {
		var certs certificateMsg
		if !certs.unmarshal(msg) {
			return nil, c.badMessage(msg, typeCertificate)
		}
		hs.transcript.Write(msg)
		if err := hs.verifyClientCertificate(certs.certificates); err != nil {
			return nil, err
		}
		if msg, err = c.readHandshake(); err != nil {
			return nil, err
		}
	}
	data, ok := parseClientKeyExchange(msg)
	if !ok {
		return nil, c.badMessage(msg, typeClientKeyExchange)
	}
	hs.transcript.Write(msg)
	preMaster, err := hs.keyExchange(data)
	if err != nil {
		return nil, err
	}
	defer clear(preMaster)
	var master []byte
	if c.extendedMasterSecret {
		master = extendedMasterSecret(preMaster, hs.transcript.Bytes())
	} else {
		master = masterSecret(preMaster, c.clientRandom, c.serverRandom)
	}
	if hs.clientKey != nil {
		if msg, err = c.readHandshake(); err != nil {
			return nil, err
		}
		var verify certificateVerify
		if !verify.unmarshal(msg) {
			return nil, c.badMessage(msg, typeCertificateVerify)
		}
		curve := hs.clientKey.Curve
		if signatureCurve(verify.signatureAlgorithm) != curve {
			return nil, c.fail(alertIllegalParameter, fmt.Errorf("btls: the client signed with the algorithm %04x that does not fit its key", verify.signatureAlgorithm))
		}
		digest := signDigest(curve, hs.transcript.Bytes())
		if !curve.Verify(hs.clientKey.Point, hashOIDDER[curve], digest, verify.signature) {
			return nil, c.fail(alertDecryptError, errors.New("btls: invalid signature by the client certificate"))
		}
		hs.transcript.Write(msg)
	}
	return master, nil
}

func (hs *serverHandshake) keyExchange(data []byte) ([]byte, error) {
	c := hs.c
	key := hs.cert.PrivateKey
	curve := key.Curve
	if hs.suite.ephemeral {
		if len(data) != 2*curve.Size() || !curve.ValidPublicKey(data) {
			return nil, c.fail(alertIllegalParameter, errors.New("btls: the ephemeral key of the client is not a point of the curve"))
		}
		preMaster, err := curve.DH(hs.ephemeral, data, 2*curve.Size())
		if err != nil {
			return nil, c.fail(alertIllegalParameter, err)
		}
		return preMaster, nil
	}
	preMaster, err := curve.KeyUnwrap(key.D, data, nil)
	if err != nil || len(preMaster) != masterSecretLength {
		return nil, c.fail(alertDecryptError, errors.New("btls: invalid key token of the client"))
	}
	return preMaster, nil
}

func (hs *serverHandshake) verifyClientCertificate(raw [][]byte) error {
	c := hs.c
	cfg := c.config
	if len(raw) == 0 {
		if cfg.ClientAuth == tls.RequireAnyClientCert || cfg.ClientAuth == tls.RequireAndVerifyClientCert {
			return c.fail(alertBadCertificate, errors.New("btls: the client did not provide a certificate"))
		}
		if cfg.VerifyPeerCertificate != nil {
			if err := cfg.VerifyPeerCertificate(nil, nil); err != nil {
				return c.fail(alertBadCertificate, err)
			}
		}
		return nil
	}
	certs := make([]*x509.Certificate, len(raw))
	for i, der := range raw {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return c.fail(alertBadCertificate, fmt.Errorf("btls: failed to parse client certificate: %w", err))
		}
		certs[i] = cert
	}
	var chains [][]*x509.Certificate
	if cfg.ClientAuth >= tls.VerifyClientCertIfGiven {
		chain, err := VerifyChain(certs, cfg.ClientCAs, cfg.now())
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
		return c.fail(alertUnsupportedCertificate, fmt.Errorf("btls: the certificate of the client has no bign key: %w", err))
	}
	c.peerCertificates, c.verifiedChains = certs, chains
	hs.clientKey = key
	return nil
}

func (hs *serverHandshake) finish(master []byte) error {
	c := hs.c
	keys := keysFromMaster(hs.suite, master, c.clientRandom, c.serverRandom)
	clientCipher, err := newRecordCipher(hs.suite, keys.clientMAC, keys.clientKey, keys.clientIV)
	if err != nil {
		return c.fail(alertInternalError, err)
	}
	serverCipher, err := newRecordCipher(hs.suite, keys.serverMAC, keys.serverKey, keys.serverIV)
	if err != nil {
		return c.fail(alertInternalError, err)
	}
	c.in.next = clientCipher
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
	expected := finishedSum(master, labelClientFinished, hs.transcript.Bytes())
	if subtle.ConstantTimeCompare(expected, verify) != 1 {
		return c.fail(alertDecryptError, errors.New("btls: the Finished of the client does not check out"))
	}
	hs.transcript.Write(msg)
	if err := c.writeChangeCipherSpec(serverCipher); err != nil {
		return err
	}
	finished := finishedSum(master, labelServerFinished, hs.transcript.Bytes())
	if err := c.writeHandshake(finishedMsg(finished), &hs.transcript); err != nil {
		return err
	}
	c.master = master
	return c.flushOut()
}
