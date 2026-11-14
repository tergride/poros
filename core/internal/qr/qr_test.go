package qr

import (
	"image"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"
)

// picture draws QR codes as a screen shows them: scale pixels a module, a quiet zone of four modules, at their places
// on a light page.
func picture(t *testing.T, width, height, scale int, codes map[image.Point]string) *image.Gray {
	t.Helper()
	page := image.NewGray(image.Rect(0, 0, width, height))
	for i := range page.Pix {
		page.Pix[i] = 0xf4
	}
	for at, text := range codes {
		rows, err := Encode(text)
		if err != nil {
			t.Fatal(err)
		}
		size := (len(rows) + 8) * scale
		for y := range size {
			for x := range size {
				page.SetGray(at.X+x, at.Y+y, color.Gray{Y: 0xff})
			}
		}
		for y, row := range rows {
			for x, module := range row {
				if module != '1' {
					continue
				}
				for dy := range scale {
					for dx := range scale {
						page.SetGray(at.X+(x+4)*scale+dx, at.Y+(y+4)*scale+dy, color.Gray{Y: 0x10})
					}
				}
			}
		}
	}
	return page
}

func TestEncodeAndReadBack(t *testing.T) {
	link := "vless://8f3a2c1e-0000-4000-8000-0000000071bd@de1.example.com:443?security=reality&sni=www.example.com#Германия · Франкфурт"
	rows, err := Encode(link)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 21 || len(rows)%4 != 1 || len(rows[0]) != len(rows) {
		t.Fatalf("a QR code is square, 21 modules and 4 more a version: %d×%d", len(rows), len(rows[0]))
	}
	// The finder in the top left corner: a ring of seven modules around a square of three.
	if rows[0][:7] != "1111111" || rows[1][:7] != "1000001" || rows[3][:7] != "1011101" {
		t.Errorf("finder: %q %q %q", rows[0][:7], rows[1][:7], rows[3][:7])
	}
	if got := Decode(picture(t, 400, 400, 3, map[image.Point]string{{20, 20}: link})); !slices.Equal(got, []string{link}) {
		t.Errorf("read back: %q", got)
	}
}

func TestDecodeFindsEveryCodeOnAScreen(t *testing.T) {
	first := "otpauth://totp/GitHub:poros-dev?secret=JBSWY3DPEHPK3PXP&issuer=GitHub"
	second := "https://sub.example.com/u/8f3a2c"
	got := Decode(picture(t, 1280, 720, 4, map[image.Point]string{{60, 80}: first, {760, 300}: second}))
	slices.Sort(got)
	want := []string{first, second}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("found %q", got)
	}
	if got := Decode(picture(t, 640, 480, 1, nil)); len(got) != 0 {
		t.Errorf("an empty page: %q", got)
	}
}

func TestEncodeRefusesTooLongText(t *testing.T) {
	if _, err := Encode(strings.Repeat("x", 3000)); err != ErrTooLong {
		t.Errorf("3000 bytes: %v", err)
	}
	if _, err := Encode(strings.Repeat("x", 2900)); err != nil {
		t.Errorf("2900 bytes fit the largest code: %v", err)
	}
	if _, err := Encode(strings.Repeat("7", 7000)); err != nil {
		t.Errorf("7000 digits fit the largest code: %v", err)
	}
	began := time.Now()
	if _, err := Encode(strings.Repeat("A", 2<<20)); err != ErrTooLong {
		t.Errorf("2 MB: %v", err)
	}
	if took := time.Since(began); took > time.Second {
		t.Errorf("2 MB took %s to refuse", took)
	}
}
