// Package qr draws QR codes as matrices of modules and reads every QR code on a picture, such as a snapshot of the
// screen.
package qr

import (
	"errors"
	"image"
	"slices"
	"strings"

	"github.com/makiuchi-d/gozxing"
	multiqr "github.com/makiuchi-d/gozxing/multi/qrcode"
	"github.com/makiuchi-d/gozxing/qrcode/decoder"
	"github.com/makiuchi-d/gozxing/qrcode/encoder"
)

// ErrTooLong is text more than the largest QR code holds.
var ErrTooLong = errors.New("the text is longer than a QR code holds")

// ErrUnsupported is a snapshot of the screen on a system Poros cannot take one on yet.
var ErrUnsupported = errors.New("this system has no snapshot of the screen yet")

// maxText is the most any QR code holds, 7089 digits: a text of more bytes fits in none. The encoder finds that out
// only after writing all of it, which takes it seconds for a long text.
const maxText = 7089

// Encode gives the modules of the QR code of text, a row of '1' and '0' each, without the quiet zone around it. The
// lowest correction holds the longest links; a screen shows the code clean.
func Encode(text string) ([]string, error) {
	if len(text) > maxText {
		return nil, ErrTooLong
	}
	code, err := encoder.Encoder_encode(text, decoder.ErrorCorrectionLevel_L, map[gozxing.EncodeHintType]interface{}{
		gozxing.EncodeHintType_CHARACTER_SET: "UTF-8",
	})
	if err != nil {
		return nil, ErrTooLong
	}
	matrix := code.GetMatrix()
	rows := make([]string, matrix.GetHeight())
	for y := range rows {
		var row strings.Builder
		for x := range matrix.GetWidth() {
			if matrix.Get(x, y) == 1 {
				row.WriteByte('1')
			} else {
				row.WriteByte('0')
			}
		}
		rows[y] = row.String()
	}
	return rows, nil
}

// Decode reads every QR code on a picture, each text once, in the order found; none is no error.
func Decode(picture image.Image) []string {
	bitmap, err := gozxing.NewBinaryBitmapFromImage(picture)
	if err != nil {
		return nil
	}
	results, err := multiqr.NewQRCodeMultiReader().DecodeMultiple(bitmap, map[gozxing.DecodeHintType]interface{}{
		gozxing.DecodeHintType_TRY_HARDER:    true,
		gozxing.DecodeHintType_CHARACTER_SET: "UTF-8",
	})
	if err != nil {
		return nil
	}
	var texts []string
	for _, result := range results {
		if text := result.GetText(); text != "" && !slices.Contains(texts, text) {
			texts = append(texts, text)
		}
	}
	return texts
}
