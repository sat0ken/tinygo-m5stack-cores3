package bmp

import (
	"bytes"
	"image/color"
	"testing"

	"golang.org/x/image/bmp"
)

func TestRoundTrip(t *testing.T) {
	w, h := 5, 3
	pix := make([]byte, w*h*2)
	// (0,0) pure red, (4,2) pure blue, (2,1) white
	set := func(x, y int, v uint16) { pix[(y*w+x)*2], pix[(y*w+x)*2+1] = byte(v>>8), byte(v) }
	set(0, 0, 0xF800)
	set(4, 2, 0x001F)
	set(2, 1, 0xFFFF)
	var buf bytes.Buffer
	if err := WriteRGB565BE(&buf, pix, w, h, make([]byte, 16)); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != Size(w, h) {
		t.Fatalf("size %d != %d", buf.Len(), Size(w, h))
	}
	img, err := bmp.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	check := func(x, y int, want color.RGBA) {
		r, g, b, _ := img.At(x, y).RGBA()
		got := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
		if got != want {
			t.Errorf("(%d,%d) = %v, want %v", x, y, got, want)
		}
	}
	check(0, 0, color.RGBA{255, 0, 0, 255})
	check(4, 2, color.RGBA{0, 0, 255, 255})
	check(2, 1, color.RGBA{255, 255, 255, 255})
	check(1, 1, color.RGBA{0, 0, 0, 255})
}
