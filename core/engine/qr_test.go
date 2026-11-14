package engine

import (
	"strings"
	"testing"
)

func TestEncodeQRTellsATooLongLink(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	code, err := e.EncodeQR(QRText{Text: "https://sub.example.com/u/8f3a2c"})
	if err != nil || len(code.Rows) < 21 || len(code.Rows[0]) != len(code.Rows) {
		t.Fatalf("a short link: %d rows, %v", len(code.Rows), err)
	}
	if _, err = e.EncodeQR(QRText{Text: "poros://add/" + strings.Repeat("A", 3000)}); reason(err) != ReasonQRTooLong {
		t.Errorf("a long link: %v", err)
	}
}
