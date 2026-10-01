package pngstream

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"testing"
)

func to565(c color.Color) (byte, byte) {
	r, g, b, a := c.RGBA() // premultiplied 16 bit
	_ = a
	r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)
	v := uint16(r8&0xF8)<<8 | uint16(g8&0xFC)<<3 | uint16(b8)>>3
	return byte(v >> 8), byte(v)
}

func check(t *testing.T, name string, data []byte, opaque bool) {
	ref, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("%s: image/png: %v", name, err)
	}
	var d Decoder
	bad := 0
	h, err := d.Decode(bytes.NewReader(data), func(y int, row []byte) error {
		for x := 0; x < len(row)/2; x++ {
			hi, lo := to565(ref.At(x, y))
			if row[x*2] != hi || row[x*2+1] != lo {
				// premultiplied rounding may differ by one step for translucent pixels
				if opaque {
					bad++
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if h.Width != ref.Bounds().Dx() || h.Height != ref.Bounds().Dy() {
		t.Fatalf("%s: size %dx%d", name, h.Width, h.Height)
	}
	if bad != 0 {
		t.Fatalf("%s: %d pixels differ", name, bad)
	}
}

func TestOfficialImages(t *testing.T) {
	for _, f := range []string{"testdata/cores3_test_picture01.png", "testdata/cores3_test_picture02.png"} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Skip("testdata not present: ", err)
		}
		check(t, f, data, true)
	}
}

func TestColorTypes(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	w, h := 37, 23
	rgba := image.NewNRGBA(image.Rect(0, 0, w, h))
	gray := image.NewGray(image.Rect(0, 0, w, h))
	pal := make(color.Palette, 200) // > 16 colors so that image/png writes bit depth 8
	for i := range pal {
		pal[i] = color.NRGBA{uint8(rnd.Intn(256)), uint8(rnd.Intn(256)), uint8(rnd.Intn(256)), 255}
	}
	paletted := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBA{uint8(x * 7), uint8(y * 11), uint8(rnd.Intn(256)), 255}
			rgba.Set(x, y, c)
			gray.SetGray(x, y, color.Gray{uint8(rnd.Intn(256))})
			paletted.SetColorIndex(x, y, uint8(rnd.Intn(200)))
		}
	}
	for name, img := range map[string]image.Image{"rgba-opaque(rgb)": rgba, "gray": gray, "paletted": paletted} {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		check(t, name, buf.Bytes(), true)
	}
}
