package btls

import "strconv"

type alert uint8

const (
	alertLevelWarning = 1
	alertLevelError   = 2
)

const (
	alertCloseNotify            alert = 0
	alertUnexpectedMessage      alert = 10
	alertBadRecordMAC           alert = 20
	alertRecordOverflow         alert = 22
	alertHandshakeFailure       alert = 40
	alertBadCertificate         alert = 42
	alertUnsupportedCertificate alert = 43
	alertCertificateExpired     alert = 45
	alertIllegalParameter       alert = 47
	alertUnknownCA              alert = 48
	alertDecodeError            alert = 50
	alertDecryptError           alert = 51
	alertProtocolVersion        alert = 70
	alertInternalError          alert = 80
	alertNoRenegotiation        alert = 100
	alertUnsupportedExtension   alert = 110
)

var alertText = map[alert]string{
	alertCloseNotify:            "close notify",
	alertUnexpectedMessage:      "unexpected message",
	alertBadRecordMAC:           "bad record MAC",
	alertRecordOverflow:         "record overflow",
	alertHandshakeFailure:       "handshake failure",
	alertBadCertificate:         "bad certificate",
	alertUnsupportedCertificate: "unsupported certificate",
	alertCertificateExpired:     "expired certificate",
	alertIllegalParameter:       "illegal parameter",
	alertUnknownCA:              "unknown certificate authority",
	alertDecodeError:            "error decoding message",
	alertDecryptError:           "error decrypting message",
	alertProtocolVersion:        "protocol version not supported",
	alertInternalError:          "internal error",
	alertNoRenegotiation:        "no renegotiation",
	alertUnsupportedExtension:   "unsupported extension",
}

func (e alert) String() string {
	if s, ok := alertText[e]; ok {
		return "btls: " + s
	}
	return "btls: alert(" + strconv.Itoa(int(e)) + ")"
}

func (e alert) Error() string { return e.String() }
