package engine

import (
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
	texts := qr.Decode(screen)
	if texts == nil {
		texts = []string{}
	}
	return QRScan{Texts: texts}, nil
}
