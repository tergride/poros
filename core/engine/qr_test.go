package engine

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"slices"
	"strings"
	"testing"
)

func TestEncodeQRTellsATooLongLink(t *testing.T) {
	e, _ := openTest(t, tempDir(t))
	code, err := e.EncodeQR(QRText{Text: "https://sub.example.com/u/8f3a2c"})
	if err != nil || len(code.Rows) < 21 || len(code.Rows[0]) != len(code.Rows) {
		t.Fatalf("a short link: %d rows, %v", len(code.Rows), err)
	}
	if _, err = e.EncodeQR(QRText{Text: "poros://add/" + strings.Repeat("A", 3000)}); reason(err) != ReasonQRTooLong {
		t.Errorf("a long link: %v", err)
	}
}

// The QR code of a preset's link, sent as a picture through a messenger that compresses it to JPEG, reads back into
// the link, and the link into the preset.
func TestReadQRBringsAPreset(t *testing.T) {
	e, _ := openTest(t, tempDir(t))
	var rules []PresetRule
	for _, site := range strings.Fields("alpha.example.com bravo.example.net charlie.example.org delta.example.com echo.example.net " +
		"foxtrot.example.org golf.example.com hotel.example.net india.example.org juliett.example.com") {
		rules = append(rules, rule(CondSuffix, site, ActionProxy))
	}
	saved, err := e.SavePreset(PresetCopy{Name: "Shared", Default: ActionDirect, Rules: rules})
	if err != nil {
		t.Fatal(err)
	}
	link, _ := e.ExportPreset(saved.ID)
	code, err := e.EncodeQR(QRText{Text: link.Link})
	if err != nil {
		t.Fatal(err)
	}
	// Six pixels a module and four modules of quiet zone, on white.
	const scale = 6
	size := (len(code.Rows) + 8) * scale
	page := image.NewGray(image.Rect(0, 0, size, size))
	for i := range page.Pix {
		page.Pix[i] = 0xff
	}
	for y, row := range code.Rows {
		for x, module := range row {
			if module == '1' {
				for d := range scale * scale {
					page.SetGray((x+4)*scale+d%scale, (y+4)*scale+d/scale, color.Gray{})
				}
			}
		}
	}
	var file bytes.Buffer
	if err = jpeg.Encode(&file, page, &jpeg.Options{Quality: 70}); err != nil {
		t.Fatal(err)
	}

	scan, err := e.ReadQR(QRImage{Image: base64.StdEncoding.EncodeToString(file.Bytes())})
	if err != nil || !slices.Equal(scan.Texts, []string{link.Link}) {
		t.Fatalf("the picture reads as %q, %v", scan.Texts, err)
	}
	imported, err := e.ImportPreset(PresetLink{Link: scan.Texts[0]})
	if err != nil || imported.Rules != len(rules) {
		t.Fatalf("the preset from the picture: %+v, %v", imported, err)
	}

	var white bytes.Buffer
	for i := range page.Pix {
		page.Pix[i] = 0xff
	}
	_ = jpeg.Encode(&white, page, nil)
	if none, err := e.ReadQR(QRImage{Image: base64.StdEncoding.EncodeToString(white.Bytes())}); err != nil || none.Texts == nil || len(none.Texts) != 0 {
		t.Errorf("a picture without a code: %#v, %v", none.Texts, err)
	}
	for _, bad := range []string{"", "!!!", base64.StdEncoding.EncodeToString([]byte("not a picture"))} {
		if _, err = e.ReadQR(QRImage{Image: bad}); reason(err) != ReasonBadPicture {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if _, err = e.ReadQR(QRImage{Image: strings.Repeat("A", maxQRImage/3*4+8)}); reason(err) != ReasonBadPicture {
		t.Errorf("a picture over the limit: %v", err)
	}
}
