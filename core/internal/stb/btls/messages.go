package btls

import "encoding/binary"

const versionTLS12 = 0x0303

const (
	typeHelloRequest       uint8 = 0
	typeClientHello        uint8 = 1
	typeServerHello        uint8 = 2
	typeCertificate        uint8 = 11
	typeServerKeyExchange  uint8 = 12
	typeCertificateRequest uint8 = 13
	typeServerHelloDone    uint8 = 14
	typeCertificateVerify  uint8 = 15
	typeClientKeyExchange  uint8 = 16
	typeFinished           uint8 = 20
)

const (
	extensionSignatureAlgorithms  uint16 = 13
	extensionExtendedMasterSecret uint16 = 23
	extensionRenegotiationInfo    uint16 = 0xff01
	scsvRenegotiation             uint16 = 0x00ff
	compressionNone               uint8  = 0
	certificateTypeBignSign       uint8  = 231
	maxHandshakeMessage                  = 1 << 16
	maxCertificateListLength             = 1<<24 - 1
)

// reader takes apart the fields of a handshake message; every method reports whether the field was there.
type reader []byte

func (r *reader) u8(out *uint8) bool {
	if len(*r) < 1 {
		return false
	}
	*out = (*r)[0]
	*r = (*r)[1:]
	return true
}

func (r *reader) u16(out *uint16) bool {
	if len(*r) < 2 {
		return false
	}
	*out = binary.BigEndian.Uint16(*r)
	*r = (*r)[2:]
	return true
}

func (r *reader) u24(out *int) bool {
	if len(*r) < 3 {
		return false
	}
	*out = int((*r)[0])<<16 | int((*r)[1])<<8 | int((*r)[2])
	*r = (*r)[3:]
	return true
}

func (r *reader) bytes(n int, out *[]byte) bool {
	if n < 0 || len(*r) < n {
		return false
	}
	*out = (*r)[:n:n]
	*r = (*r)[n:]
	return true
}

func (r *reader) vec8(out *[]byte) bool {
	var n uint8
	return r.u8(&n) && r.bytes(int(n), out)
}

func (r *reader) vec16(out *[]byte) bool {
	var n uint16
	return r.u16(&n) && r.bytes(int(n), out)
}

func (r *reader) vec24(out *[]byte) bool {
	var n int
	return r.u24(&n) && r.bytes(n, out)
}

func (r *reader) empty() bool { return len(*r) == 0 }

// appendVec appends a vector with a length prefix of lenBytes bytes; f appends the contents.
func appendVec(b []byte, lenBytes int, f func([]byte) []byte) []byte {
	start := len(b)
	b = append(b, make([]byte, lenBytes)...)
	b = f(b)
	n := len(b) - start - lenBytes
	for i := lenBytes - 1; i >= 0; i-- {
		b[start+i] = byte(n)
		n >>= 8
	}
	return b
}

func appendBytesVec(b []byte, lenBytes int, data []byte) []byte {
	return appendVec(b, lenBytes, func(b []byte) []byte { return append(b, data...) })
}

// handshakeMessage frames a body as a message of the type.
func handshakeMessage(typ uint8, body []byte) []byte {
	msg := make([]byte, 4, 4+len(body))
	msg[0] = typ
	msg[1], msg[2], msg[3] = byte(len(body)>>16), byte(len(body)>>8), byte(len(body))
	return append(msg, body...)
}

// messageBody checks the type of a framed message and returns its body.
func messageBody(msg []byte, typ uint8) (reader, bool) {
	if len(msg) < 4 || msg[0] != typ {
		return nil, false
	}
	return reader(msg[4:]), true
}

type clientHello struct {
	version             uint16
	random              []byte
	sessionID           []byte
	cipherSuites        []uint16
	compressionMethods  []uint8
	signatureAlgorithms []uint16
	// secureRenegotiationSupported is set by the extension renegotiation_info or by the signaling suite.
	secureRenegotiationSupported bool
	secureRenegotiation          []byte
	extendedMasterSecret         bool
}

func (m *clientHello) marshal() []byte {
	b := binary.BigEndian.AppendUint16(nil, m.version)
	b = append(b, m.random...)
	b = appendBytesVec(b, 1, m.sessionID)
	b = appendVec(b, 2, func(b []byte) []byte {
		for _, s := range m.cipherSuites {
			b = binary.BigEndian.AppendUint16(b, s)
		}
		return b
	})
	b = appendBytesVec(b, 1, m.compressionMethods)
	b = appendVec(b, 2, func(b []byte) []byte {
		if len(m.signatureAlgorithms) > 0 {
			b = binary.BigEndian.AppendUint16(b, extensionSignatureAlgorithms)
			b = appendVec(b, 2, func(b []byte) []byte {
				return appendVec(b, 2, func(b []byte) []byte {
					for _, alg := range m.signatureAlgorithms {
						b = binary.BigEndian.AppendUint16(b, alg)
					}
					return b
				})
			})
		}
		if m.extendedMasterSecret {
			b = binary.BigEndian.AppendUint16(b, extensionExtendedMasterSecret)
			b = appendBytesVec(b, 2, nil)
		}
		if m.secureRenegotiationSupported {
			b = binary.BigEndian.AppendUint16(b, extensionRenegotiationInfo)
			b = appendVec(b, 2, func(b []byte) []byte { return appendBytesVec(b, 1, m.secureRenegotiation) })
		}
		return b
	})
	return handshakeMessage(typeClientHello, b)
}

func (m *clientHello) unmarshal(msg []byte) bool {
	r, ok := messageBody(msg, typeClientHello)
	if !ok {
		return false
	}
	var suites, compression []byte
	if !r.u16(&m.version) || !r.bytes(randomLength, &m.random) || !r.vec8(&m.sessionID) || len(m.sessionID) > 32 ||
		!r.vec16(&suites) || len(suites)%2 != 0 || !r.vec8(&compression) || len(compression) == 0 {
		return false
	}
	for s := reader(suites); !s.empty(); {
		var id uint16
		s.u16(&id)
		if id == scsvRenegotiation {
			m.secureRenegotiationSupported = true
		}
		m.cipherSuites = append(m.cipherSuites, id)
	}
	m.compressionMethods = compression
	if r.empty() {
		return true
	}
	var extensions []byte
	if !r.vec16(&extensions) || !r.empty() {
		return false
	}
	seen := map[uint16]bool{}
	for e := reader(extensions); !e.empty(); {
		var typ uint16
		var data []byte
		if !e.u16(&typ) || !e.vec16(&data) || seen[typ] {
			return false
		}
		seen[typ] = true
		d := reader(data)
		switch typ {
		case extensionSignatureAlgorithms:
			var list []byte
			if !d.vec16(&list) || len(list)%2 != 0 || len(list) == 0 || !d.empty() {
				return false
			}
			for l := reader(list); !l.empty(); {
				var alg uint16
				l.u16(&alg)
				m.signatureAlgorithms = append(m.signatureAlgorithms, alg)
			}
		case extensionExtendedMasterSecret:
			if !d.empty() {
				return false
			}
			m.extendedMasterSecret = true
		case extensionRenegotiationInfo:
			if !d.vec8(&m.secureRenegotiation) || !d.empty() {
				return false
			}
			m.secureRenegotiationSupported = true
		}
	}
	return true
}

type serverHello struct {
	version                      uint16
	random                       []byte
	sessionID                    []byte
	cipherSuite                  uint16
	compressionMethod            uint8
	secureRenegotiationSupported bool
	secureRenegotiation          []byte
	extendedMasterSecret         bool
	// unknownExtension is set when the server answered with an extension the client never offers.
	unknownExtension bool
}

func (m *serverHello) marshal() []byte {
	b := binary.BigEndian.AppendUint16(nil, m.version)
	b = append(b, m.random...)
	b = appendBytesVec(b, 1, m.sessionID)
	b = binary.BigEndian.AppendUint16(b, m.cipherSuite)
	b = append(b, m.compressionMethod)
	if m.extendedMasterSecret || m.secureRenegotiationSupported {
		b = appendVec(b, 2, func(b []byte) []byte {
			if m.extendedMasterSecret {
				b = binary.BigEndian.AppendUint16(b, extensionExtendedMasterSecret)
				b = appendBytesVec(b, 2, nil)
			}
			if m.secureRenegotiationSupported {
				b = binary.BigEndian.AppendUint16(b, extensionRenegotiationInfo)
				b = appendVec(b, 2, func(b []byte) []byte { return appendBytesVec(b, 1, m.secureRenegotiation) })
			}
			return b
		})
	}
	return handshakeMessage(typeServerHello, b)
}

func (m *serverHello) unmarshal(msg []byte) bool {
	r, ok := messageBody(msg, typeServerHello)
	if !ok {
		return false
	}
	if !r.u16(&m.version) || !r.bytes(randomLength, &m.random) || !r.vec8(&m.sessionID) || len(m.sessionID) > 32 ||
		!r.u16(&m.cipherSuite) || !r.u8(&m.compressionMethod) {
		return false
	}
	if r.empty() {
		return true
	}
	var extensions []byte
	if !r.vec16(&extensions) || !r.empty() {
		return false
	}
	seen := map[uint16]bool{}
	for e := reader(extensions); !e.empty(); {
		var typ uint16
		var data []byte
		if !e.u16(&typ) || !e.vec16(&data) || seen[typ] {
			return false
		}
		seen[typ] = true
		d := reader(data)
		switch typ {
		case extensionExtendedMasterSecret:
			if !d.empty() {
				return false
			}
			m.extendedMasterSecret = true
		case extensionRenegotiationInfo:
			if !d.vec8(&m.secureRenegotiation) || !d.empty() {
				return false
			}
			m.secureRenegotiationSupported = true
		default:
			m.unknownExtension = true
		}
	}
	return true
}

type certificateMsg struct {
	certificates [][]byte
}

func (m *certificateMsg) marshal() []byte {
	b := appendVec(nil, 3, func(b []byte) []byte {
		for _, c := range m.certificates {
			b = appendBytesVec(b, 3, c)
		}
		return b
	})
	return handshakeMessage(typeCertificate, b)
}

func (m *certificateMsg) unmarshal(msg []byte) bool {
	r, ok := messageBody(msg, typeCertificate)
	if !ok {
		return false
	}
	var list []byte
	if !r.vec24(&list) || !r.empty() {
		return false
	}
	for l := reader(list); !l.empty(); {
		var cert []byte
		if !l.vec24(&cert) || len(cert) == 0 {
			return false
		}
		m.certificates = append(m.certificates, cert)
	}
	return true
}

// serverKeyExchange of a DHE suite: the ephemeral public key with a length of one byte, then the signature of
// client_random ‖ server_random ‖ the key with its length.
type serverKeyExchange struct {
	params             []byte
	key                []byte
	signatureAlgorithm uint16
	signature          []byte
}

func (m *serverKeyExchange) marshal() []byte {
	b := append([]byte(nil), m.params...)
	b = binary.BigEndian.AppendUint16(b, m.signatureAlgorithm)
	b = appendBytesVec(b, 2, m.signature)
	return handshakeMessage(typeServerKeyExchange, b)
}

func (m *serverKeyExchange) unmarshal(msg []byte) bool {
	r, ok := messageBody(msg, typeServerKeyExchange)
	if !ok {
		return false
	}
	start := r
	if !r.vec8(&m.key) || len(m.key) == 0 {
		return false
	}
	m.params = start[:1+len(m.key)]
	return r.u16(&m.signatureAlgorithm) && r.vec16(&m.signature) && r.empty()
}

func dhParams(key []byte) []byte {
	return appendBytesVec(nil, 1, key)
}

type certificateRequest struct {
	certificateTypes    []uint8
	signatureAlgorithms []uint16
	authorities         [][]byte
}

func (m *certificateRequest) marshal() []byte {
	b := appendBytesVec(nil, 1, m.certificateTypes)
	b = appendVec(b, 2, func(b []byte) []byte {
		for _, alg := range m.signatureAlgorithms {
			b = binary.BigEndian.AppendUint16(b, alg)
		}
		return b
	})
	b = appendVec(b, 2, func(b []byte) []byte {
		for _, a := range m.authorities {
			b = appendBytesVec(b, 2, a)
		}
		return b
	})
	return handshakeMessage(typeCertificateRequest, b)
}

func (m *certificateRequest) unmarshal(msg []byte) bool {
	r, ok := messageBody(msg, typeCertificateRequest)
	if !ok {
		return false
	}
	var algs, authorities []byte
	if !r.vec8(&m.certificateTypes) || !r.vec16(&algs) || len(algs)%2 != 0 || !r.vec16(&authorities) || !r.empty() {
		return false
	}
	for l := reader(algs); !l.empty(); {
		var alg uint16
		l.u16(&alg)
		m.signatureAlgorithms = append(m.signatureAlgorithms, alg)
	}
	for l := reader(authorities); !l.empty(); {
		var dn []byte
		if !l.vec16(&dn) {
			return false
		}
		m.authorities = append(m.authorities, dn)
	}
	return true
}

func isEmptyMessage(msg []byte, typ uint8) bool {
	r, ok := messageBody(msg, typ)
	return ok && r.empty()
}

// clientKeyExchange carries the ephemeral public key of DHE or the key token of DHT, with a length of one byte.
func clientKeyExchangeMsg(data []byte) []byte {
	return handshakeMessage(typeClientKeyExchange, appendBytesVec(nil, 1, data))
}

func parseClientKeyExchange(msg []byte) ([]byte, bool) {
	r, ok := messageBody(msg, typeClientKeyExchange)
	if !ok {
		return nil, false
	}
	var data []byte
	if !r.vec8(&data) || !r.empty() || len(data) == 0 {
		return nil, false
	}
	return data, true
}

type certificateVerify struct {
	signatureAlgorithm uint16
	signature          []byte
}

func (m *certificateVerify) marshal() []byte {
	b := binary.BigEndian.AppendUint16(nil, m.signatureAlgorithm)
	b = appendBytesVec(b, 2, m.signature)
	return handshakeMessage(typeCertificateVerify, b)
}

func (m *certificateVerify) unmarshal(msg []byte) bool {
	r, ok := messageBody(msg, typeCertificateVerify)
	return ok && r.u16(&m.signatureAlgorithm) && r.vec16(&m.signature) && r.empty()
}

func finishedMsg(verifyData []byte) []byte {
	return handshakeMessage(typeFinished, verifyData)
}

func parseFinished(msg []byte) ([]byte, bool) {
	r, ok := messageBody(msg, typeFinished)
	if !ok || len(r) != finishedLength {
		return nil, false
	}
	return r, true
}
