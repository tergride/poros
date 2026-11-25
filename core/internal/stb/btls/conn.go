package btls

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	recordHeaderLength = 5
	maxPlaintext       = 16384
	maxCiphertext      = maxPlaintext + 2048
	// maxUselessRecords bounds the warnings and empty records a peer may send in a row.
	maxUselessRecords = 16
	// versionHello is the record version of the first ClientHello, which older servers expect.
	versionHello = 0x0301
)

// Conn is a BTLS connection over a stream.
type Conn struct {
	conn     net.Conn
	isClient bool
	config   *Config

	handshakeMu       sync.Mutex
	handshakeErr      error
	handshakeComplete atomic.Bool
	// activeCall counts Writes in flight by twos; bit 0 is set by Close.
	activeCall atomic.Int32

	// The handshake sets these once.
	vers                 uint16
	suite                *cipherSuite
	master               []byte
	clientRandom         []byte
	serverRandom         []byte
	extendedMasterSecret bool
	peerCertificates     []*x509.Certificate
	verifiedChains       [][]*x509.Certificate

	in, out  halfConn
	rawInput bytes.Buffer
	input    []byte
	hand     bytes.Buffer
	useless  int

	sendBuf   []byte
	buffering bool

	closeNotifySent bool
	closeNotifyErr  error
}

type halfConn struct {
	sync.Mutex
	err    error
	cipher recordCipher
	next   recordCipher
	seq    uint64
}

func (hc *halfConn) setErrorLocked(err error) error {
	hc.err = err
	return err
}

func (hc *halfConn) changeCipherSpec() error {
	if hc.next == nil {
		return alertInternalError
	}
	hc.cipher, hc.next, hc.seq = hc.next, nil, 0
	return nil
}

func (hc *halfConn) incSeq() error {
	hc.seq++
	if hc.seq == 0 {
		return errors.New("btls: record sequence number wrapped")
	}
	return nil
}

func (hc *halfConn) encrypt(typ recordType, plaintext []byte) ([]byte, error) {
	if hc.cipher == nil {
		return plaintext, nil
	}
	out := hc.cipher.seal(hc.seq, typ, plaintext)
	return out, hc.incSeq()
}

func (hc *halfConn) decrypt(typ recordType, payload []byte) ([]byte, error) {
	if hc.cipher == nil {
		return append([]byte(nil), payload...), nil
	}
	plain, err := hc.cipher.open(hc.seq, typ, payload)
	if err != nil {
		return nil, err
	}
	return plain, hc.incSeq()
}

// Client returns a client side of a BTLS connection over conn.
func Client(conn net.Conn, config *Config) *Conn {
	return &Conn{conn: conn, isClient: true, config: config}
}

// Server returns a server side of a BTLS connection over conn; the config needs a certificate.
func Server(conn net.Conn, config *Config) *Conn {
	return &Conn{conn: conn, config: config}
}

func (c *Conn) LocalAddr() net.Addr                { return c.conn.LocalAddr() }
func (c *Conn) RemoteAddr() net.Addr               { return c.conn.RemoteAddr() }
func (c *Conn) SetDeadline(t time.Time) error      { return c.conn.SetDeadline(t) }
func (c *Conn) SetReadDeadline(t time.Time) error  { return c.conn.SetReadDeadline(t) }
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.conn.SetWriteDeadline(t) }

// NetConn returns the connection BTLS runs over.
func (c *Conn) NetConn() net.Conn { return c.conn }

// atLeastReader reads until N bytes came, then reports io.EOF so that bytes.Buffer.ReadFrom stops.
type atLeastReader struct {
	r io.Reader
	n int64
}

func (r *atLeastReader) Read(p []byte) (int, error) {
	if r.n <= 0 {
		return 0, io.EOF
	}
	n, err := r.r.Read(p)
	r.n -= int64(n)
	if r.n > 0 && err == io.EOF {
		return n, io.ErrUnexpectedEOF
	}
	if r.n <= 0 && err == nil {
		return n, io.EOF
	}
	return n, err
}

func (c *Conn) readFromUntil(n int) error {
	if c.rawInput.Len() >= n {
		return nil
	}
	needs := n - c.rawInput.Len()
	c.rawInput.Grow(needs + bytes.MinRead)
	_, err := c.rawInput.ReadFrom(&atLeastReader{c.conn, int64(needs)})
	return err
}

// readFailed keeps a read error, except a timeout, after which reading may go on.
func (c *Conn) readFailed(err error) error {
	if e, ok := err.(net.Error); ok && e.Timeout() {
		return err
	}
	return c.in.setErrorLocked(err)
}

func (c *Conn) readRecord() error { return c.readRecordOrCCS(false) }

func (c *Conn) readChangeCipherSpec() error { return c.readRecordOrCCS(true) }

// readRecordOrCCS reads one record; c.in must be locked. Handshake data go to c.hand, application data to c.input.
func (c *Conn) readRecordOrCCS(expectChangeCipherSpec bool) error {
	if c.in.err != nil {
		return c.in.err
	}
	if err := c.readFromUntil(recordHeaderLength); err != nil {
		// A peer may close the stream without close_notify; that is fine only between records.
		if err == io.ErrUnexpectedEOF && c.rawInput.Len() == 0 {
			err = io.EOF
		}
		return c.readFailed(err)
	}
	header := c.rawInput.Bytes()[:recordHeaderLength]
	typ := recordType(header[0])
	vers := uint16(header[1])<<8 | uint16(header[2])
	n := int(header[3])<<8 | int(header[4])
	if c.vers == 0 {
		if vers>>8 != 3 || typ != recordTypeHandshake && typ != recordTypeAlert {
			c.sendAlert(alertProtocolVersion)
			return c.in.setErrorLocked(errors.New("btls: the first record does not look like a TLS handshake"))
		}
	} else if vers != c.vers {
		c.sendAlert(alertProtocolVersion)
		return c.in.setErrorLocked(fmt.Errorf("btls: received record with version %x when expecting version %x", vers, c.vers))
	}
	if n > maxCiphertext {
		c.sendAlert(alertRecordOverflow)
		return c.in.setErrorLocked(fmt.Errorf("btls: oversized record received with length %d", n))
	}
	if err := c.readFromUntil(recordHeaderLength + n); err != nil {
		return c.readFailed(err)
	}
	record := c.rawInput.Next(recordHeaderLength + n)
	data, err := c.in.decrypt(typ, record[recordHeaderLength:])
	if err != nil {
		return c.in.setErrorLocked(c.sendAlert(alertBadRecordMAC))
	}
	if len(data) > maxPlaintext {
		return c.in.setErrorLocked(c.sendAlert(alertRecordOverflow))
	}
	if c.in.cipher == nil && typ == recordTypeApplicationData {
		return c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
	}
	switch typ {
	case recordTypeAlert:
		if len(data) != 2 {
			return c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
		}
		if alert(data[1]) == alertCloseNotify {
			return c.in.setErrorLocked(io.EOF)
		}
		switch data[0] {
		case alertLevelWarning:
			return c.retryReadRecord(expectChangeCipherSpec)
		case alertLevelError:
			return c.in.setErrorLocked(&net.OpError{Op: "remote error", Err: alert(data[1])})
		}
		return c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
	case recordTypeChangeCipherSpec:
		if len(data) != 1 || data[0] != 1 {
			return c.in.setErrorLocked(c.sendAlert(alertDecodeError))
		}
		if !expectChangeCipherSpec || c.hand.Len() > 0 {
			return c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
		}
		if err := c.in.changeCipherSpec(); err != nil {
			return c.in.setErrorLocked(c.sendAlert(alertInternalError))
		}
	case recordTypeApplicationData:
		if !c.handshakeComplete.Load() || expectChangeCipherSpec || c.hand.Len() > 0 {
			return c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
		}
		if len(data) == 0 {
			return c.retryReadRecord(expectChangeCipherSpec)
		}
		c.useless = 0
		c.input = data
	case recordTypeHandshake:
		if len(data) == 0 || expectChangeCipherSpec {
			return c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
		}
		c.useless = 0
		c.hand.Write(data)
	default:
		return c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
	}
	return nil
}

func (c *Conn) retryReadRecord(expectChangeCipherSpec bool) error {
	c.useless++
	if c.useless > maxUselessRecords {
		c.sendAlert(alertUnexpectedMessage)
		return c.in.setErrorLocked(errors.New("btls: too many ignored records"))
	}
	return c.readRecordOrCCS(expectChangeCipherSpec)
}

// readHandshake returns the next whole handshake message with its header; c.in must be locked.
func (c *Conn) readHandshake() ([]byte, error) {
	for c.hand.Len() < 4 {
		if err := c.readRecord(); err != nil {
			return nil, err
		}
	}
	data := c.hand.Bytes()
	n := int(data[1])<<16 | int(data[2])<<8 | int(data[3])
	if n > maxHandshakeMessage {
		c.sendAlert(alertInternalError)
		return nil, c.in.setErrorLocked(fmt.Errorf("btls: handshake message of length %d bytes exceeds maximum of %d bytes", n, maxHandshakeMessage))
	}
	for c.hand.Len() < 4+n {
		if err := c.readRecord(); err != nil {
			return nil, err
		}
	}
	msg := make([]byte, 4+n)
	copy(msg, c.hand.Next(4+n))
	return msg, nil
}

// writeRecordLocked splits data into records of the type; c.out must be locked. Unless the handshake buffers a
// flight, the records go out at once.
func (c *Conn) writeRecordLocked(typ recordType, data []byte) (int, error) {
	if c.out.err != nil {
		return 0, c.out.err
	}
	vers := c.vers
	if vers == 0 {
		vers = versionHello
	}
	n := 0
	for {
		m := min(len(data), maxPlaintext)
		payload, err := c.out.encrypt(typ, data[:m])
		if err != nil {
			return n, c.out.setErrorLocked(err)
		}
		c.sendBuf = append(c.sendBuf, byte(typ), byte(vers>>8), byte(vers), byte(len(payload)>>8), byte(len(payload)))
		c.sendBuf = append(c.sendBuf, payload...)
		n += m
		data = data[m:]
		if len(data) == 0 {
			break
		}
	}
	if typ == recordTypeChangeCipherSpec {
		if err := c.out.changeCipherSpec(); err != nil {
			return n, c.out.setErrorLocked(err)
		}
	}
	if !c.buffering {
		if err := c.flush(); err != nil {
			return n, err
		}
	}
	return n, nil
}

// flush sends the buffered records; c.out must be locked.
func (c *Conn) flush() error {
	if len(c.sendBuf) == 0 {
		return nil
	}
	_, err := c.conn.Write(c.sendBuf)
	c.sendBuf = nil
	if err != nil {
		return c.out.setErrorLocked(err)
	}
	return nil
}

func (c *Conn) setBuffering(on bool) error {
	c.out.Lock()
	defer c.out.Unlock()
	c.buffering = on
	if !on {
		return c.flush()
	}
	return nil
}

func (c *Conn) flushOut() error {
	c.out.Lock()
	defer c.out.Unlock()
	return c.flush()
}

func (c *Conn) sendAlertLocked(err alert) error {
	level := byte(alertLevelError)
	if err == alertCloseNotify || err == alertNoRenegotiation {
		level = alertLevelWarning
	}
	_, writeErr := c.writeRecordLocked(recordTypeAlert, []byte{level, byte(err)})
	if writeErr == nil {
		writeErr = c.flush()
	}
	if level == alertLevelWarning {
		return writeErr
	}
	return c.out.setErrorLocked(&net.OpError{Op: "local error", Err: err})
}

func (c *Conn) sendAlert(err alert) error {
	c.out.Lock()
	defer c.out.Unlock()
	return c.sendAlertLocked(err)
}

// fail sends a fatal alert and returns the cause of the failure.
func (c *Conn) fail(a alert, cause error) error {
	c.sendAlert(a)
	if cause == nil {
		return a
	}
	return cause
}

func (c *Conn) badMessage(msg []byte, want uint8) error {
	if len(msg) > 0 && msg[0] != want {
		return c.fail(alertUnexpectedMessage, fmt.Errorf("btls: unexpected handshake message of type %d, want %d", msg[0], want))
	}
	return c.fail(alertDecodeError, fmt.Errorf("btls: malformed handshake message of type %d", want))
}

func (c *Conn) writeHandshake(msg []byte, transcript *bytes.Buffer) error {
	transcript.Write(msg)
	c.out.Lock()
	defer c.out.Unlock()
	_, err := c.writeRecordLocked(recordTypeHandshake, msg)
	return err
}

func (c *Conn) writeChangeCipherSpec(next recordCipher) error {
	c.out.Lock()
	defer c.out.Unlock()
	c.out.next = next
	_, err := c.writeRecordLocked(recordTypeChangeCipherSpec, []byte{1})
	return err
}

// Handshake runs the handshake unless it already ran; Read and Write run it on their own.
func (c *Conn) Handshake() error {
	return c.HandshakeContext(context.Background())
}

// HandshakeContext runs the handshake; when the context ends first, it closes the connection.
func (c *Conn) HandshakeContext(ctx context.Context) (ret error) {
	if c.handshakeComplete.Load() {
		return nil
	}
	if ctx.Done() != nil {
		done := make(chan struct{})
		interrupted := make(chan error, 1)
		defer func() {
			close(done)
			if err := <-interrupted; err != nil {
				ret = err
			}
		}()
		go func() {
			select {
			case <-ctx.Done():
				_ = c.conn.Close()
				interrupted <- ctx.Err()
			case <-done:
				interrupted <- nil
			}
		}()
	}
	c.handshakeMu.Lock()
	defer c.handshakeMu.Unlock()
	if err := c.handshakeErr; err != nil {
		return err
	}
	if c.handshakeComplete.Load() {
		return nil
	}
	c.in.Lock()
	defer c.in.Unlock()
	if c.isClient {
		c.handshakeErr = c.clientHandshake()
	} else {
		c.handshakeErr = c.serverHandshake()
	}
	if c.handshakeErr == nil {
		c.handshakeComplete.Store(true)
		c.handshakeErr = c.setBuffering(false)
	} else {
		_ = c.setBuffering(false)
	}
	return c.handshakeErr
}

// Read reads application data.
func (c *Conn) Read(b []byte) (int, error) {
	if err := c.Handshake(); err != nil {
		return 0, err
	}
	if len(b) == 0 {
		return 0, nil
	}
	c.in.Lock()
	defer c.in.Unlock()
	for len(c.input) == 0 {
		if err := c.readRecord(); err != nil {
			return 0, err
		}
		for c.hand.Len() > 0 {
			if err := c.handlePostHandshake(); err != nil {
				return 0, err
			}
		}
	}
	n := copy(b, c.input)
	c.input = c.input[n:]
	return n, nil
}

// handlePostHandshake declines a renegotiation and rejects any other handshake message.
func (c *Conn) handlePostHandshake() error {
	msg, err := c.readHandshake()
	if err != nil {
		return err
	}
	if c.isClient && msg[0] == typeHelloRequest && len(msg) == 4 || !c.isClient && msg[0] == typeClientHello {
		return c.sendAlert(alertNoRenegotiation)
	}
	c.sendAlert(alertUnexpectedMessage)
	return c.in.setErrorLocked(errors.New("btls: unexpected handshake message after the handshake"))
}

var errShutdown = errors.New("btls: protocol is shutdown")

// Write writes application data.
func (c *Conn) Write(b []byte) (int, error) {
	for {
		x := c.activeCall.Load()
		if x&1 != 0 {
			return 0, net.ErrClosed
		}
		if c.activeCall.CompareAndSwap(x, x+2) {
			break
		}
	}
	defer c.activeCall.Add(-2)
	if err := c.Handshake(); err != nil {
		return 0, err
	}
	c.out.Lock()
	defer c.out.Unlock()
	if err := c.out.err; err != nil {
		return 0, err
	}
	if c.closeNotifySent {
		return 0, errShutdown
	}
	if len(b) == 0 {
		return 0, nil
	}
	return c.writeRecordLocked(recordTypeApplicationData, b)
}

// Close sends close_notify after a finished handshake and closes the connection. A Close during a Write only
// closes the connection, as the Write may hold the lock close_notify needs.
func (c *Conn) Close() error {
	var x int32
	for {
		x = c.activeCall.Load()
		if x&1 != 0 {
			return net.ErrClosed
		}
		if c.activeCall.CompareAndSwap(x, x|1) {
			break
		}
	}
	if x != 0 {
		return c.conn.Close()
	}
	var alertErr error
	if c.handshakeComplete.Load() {
		if err := c.closeNotify(); err != nil {
			alertErr = fmt.Errorf("btls: failed to send close_notify (the connection is closed anyway): %w", err)
		}
	}
	if err := c.conn.Close(); err != nil {
		return err
	}
	return alertErr
}

// CloseWrite sends close_notify and keeps reading open.
func (c *Conn) CloseWrite() error {
	if !c.handshakeComplete.Load() {
		return errors.New("btls: CloseWrite before the handshake completed")
	}
	return c.closeNotify()
}

func (c *Conn) closeNotify() error {
	c.out.Lock()
	defer c.out.Unlock()
	if !c.closeNotifySent {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		c.closeNotifyErr = c.sendAlertLocked(alertCloseNotify)
		c.closeNotifySent = true
		_ = c.SetWriteDeadline(time.Now())
	}
	return c.closeNotifyErr
}

// ConnectionState describes a connection.
type ConnectionState struct {
	HandshakeComplete    bool
	CipherSuite          uint16
	ExtendedMasterSecret bool
	// PeerCertificates are the certificates the peer sent, leaf first.
	PeerCertificates []*x509.Certificate
	// VerifiedChains holds the chain VerifyChain built, unless the check was skipped.
	VerifiedChains [][]*x509.Certificate
}

// ConnectionState returns the state of the connection.
func (c *Conn) ConnectionState() ConnectionState {
	c.handshakeMu.Lock()
	defer c.handshakeMu.Unlock()
	state := ConnectionState{HandshakeComplete: c.handshakeComplete.Load()}
	if state.HandshakeComplete {
		state.CipherSuite = c.suite.id
		state.ExtendedMasterSecret = c.extendedMasterSecret
		state.PeerCertificates = c.peerCertificates
		state.VerifiedChains = c.verifiedChains
	}
	return state
}

// ExportKeyingMaterial returns keying material of RFC 5705 from the session.
func (c *Conn) ExportKeyingMaterial(label string, context []byte, length int) ([]byte, error) {
	if !c.handshakeComplete.Load() {
		return nil, errors.New("btls: the handshake has not completed")
	}
	return exportKeyingMaterial(c.master, c.clientRandom, c.serverRandom, label, context, length)
}
