package engine

import (
	"encoding/base64"
	"errors"

	"PorosCore/internal/jsonrpc"
	"PorosCore/internal/qr"
)

func (e *Engine) EncodeQR(p QRText) (QRCode, error) {
	rows, err := qr.Encode(p.Text)
	if errors.Is(err, qr.ErrTooLong) {
		return QRCode{}, jsonrpc.Failed(ReasonQRTooLong, "%v", err)
	}
	return QRCode{Rows: rows}, err
}

// ScanQR reads the QR codes on all monitors. The snapshot never leaves the core.
func (e *Engine) ScanQR() (QRScan, error) {
	screen, err := qr.Screen()
	if errors.Is(err, qr.ErrUnsupported) {
		return QRScan{}, jsonrpc.Failed(ReasonScanUnsupported, "%v", err)
	}
	if err != nil {
		return QRScan{}, jsonrpc.Failed(ReasonScanFailed, "%v", err)
	}
	return QRScan{Texts: nonNil(qr.Decode(screen))}, nil
}

// maxQRImage bounds the file of a picture to read, 32 MB: a photo of a phone is a few megabytes.
const maxQRImage = 32 << 20

// ReadQR reads the QR codes on a picture.
func (e *Engine) ReadQR(p QRImage) (QRScan, error) {
	if base64.StdEncoding.DecodedLen(len(p.Image)) > maxQRImage {
		return QRScan{}, jsonrpc.Failed(ReasonBadPicture, "the picture is larger than %d MB", maxQRImage>>20)
	}
	data, err := base64.StdEncoding.DecodeString(p.Image)
	if err != nil {
		return QRScan{}, jsonrpc.Failed(ReasonBadPicture, "the picture is no base64")
	}
	picture, err := qr.Picture(data)
	if err != nil {
		return QRScan{}, jsonrpc.Failed(ReasonBadPicture, "%v", err)
	}
	return QRScan{Texts: nonNil(qr.Decode(picture))}, nil
}
