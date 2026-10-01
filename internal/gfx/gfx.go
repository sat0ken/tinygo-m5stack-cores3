// Package gfx has small drawing helpers built on FillRectangle565, which is
// much faster on a SPI LCD than drawing pixel by pixel through SetPixel.
package gfx

import (
	"unsafe"

	"tinygo.org/x/tinyfont"
)

// Filler is a display that can fill rectangles with an RGB565 color, clipped
// to the screen (ili9342.Device).
type Filler interface {
	Size() (x, y int16)
	FillRectangle565(x, y, w, h int16, c uint16) error
}

// RGB565 colors used by the M5 examples.
// ref: M5GFX src/lgfx/v1/misc/colortype.hpp (TFT_BLACK, TFT_RED, TFT_GREEN, ...)
const (
	Black  = 0x0000
	White  = 0xFFFF
	Red    = 0xF800
	Green  = 0x07E0
	Blue   = 0x001F
	Yellow = 0xFFE0
)

// FillCircle draws a filled circle as horizontal spans.
func FillCircle(d Filler, x0, y0, r int16, c uint16) {
	rr := int32(r) * int32(r)
	dx := r
	for dy := int16(0); dy <= r; dy++ {
		for dx > 0 && int32(dx)*int32(dx)+int32(dy)*int32(dy) > rr {
			dx--
		}
		d.FillRectangle565(x0-dx, y0+dy, 2*dx+1, 1, c)
		if dy != 0 {
			d.FillRectangle565(x0-dx, y0-dy, 2*dx+1, 1, c)
		}
	}
}

// DrawText draws str with its baseline at y, starting at x. Runs of set
// pixels in a glyph row are drawn as one rectangle.
func DrawText(d Filler, font *tinyfont.Font, x, y int16, str string, c uint16) {
	for _, r := range str {
		g := glyph(font, r)
		drawGlyph(d, g, x, y, c)
		x += int16(g.XAdvance)
	}
}

// TextWidth returns the sum of the advances of str.
func TextWidth(font *tinyfont.Font, str string) int16 {
	w := int16(0)
	for _, r := range str {
		w += int16(glyph(font, r).XAdvance)
	}
	return w
}

// glyph looks up r without going through the Glypher interface (which would
// allocate for every character).
func glyph(font *tinyfont.Font, r rune) *tinyfont.Glyph {
	s, e := 0, len(font.Glyphs)-1
	for s <= e {
		m := (s + e) / 2
		switch g := &font.Glyphs[m]; {
		case g.Rune == r:
			return g
		case g.Rune < r:
			s = m + 1
		default:
			e = m - 1
		}
	}
	return &font.EmptyGlyph
}

func drawGlyph(d Filler, g *tinyfont.Glyph, x, y int16, c uint16) {
	bitIdx := 0
	gx := x + int16(g.XOffset)
	gy := y + int16(g.YOffset)
	for j := int16(0); j < int16(g.Height); j++ {
		run := int16(-1)
		for i := int16(0); i < int16(g.Width); i++ {
			on := false
			if b := bitIdx >> 3; b < len(g.Bitmaps) {
				on = g.Bitmaps[b]&(0x80>>(bitIdx&7)) != 0
			}
			bitIdx++
			if on && run < 0 {
				run = i
			} else if !on && run >= 0 {
				d.FillRectangle565(gx+run, gy+j, i-run, 1, c)
				run = -1
			}
		}
		if run >= 0 {
			d.FillRectangle565(gx+run, gy+j, int16(g.Width)-run, 1, c)
		}
	}
}

// DrawTextCentered draws str centered on (cx, cy), like M5GFX
// setTextDatum(middle_center): the horizontal center of the text advance and
// the vertical center of the font bounding box.
func DrawTextCentered(d Filler, font *tinyfont.Font, cx, cy int16, str string, c uint16) {
	w := TextWidth(font, str)
	// BBox = width, height, minX, minY; minY is the top of the box relative
	// to the baseline (negative).
	h := int16(font.BBox[1])
	top := cy - h/2
	DrawText(d, font, cx-int16(w)/2, top-int16(font.BBox[3]), str, c)
}

// Itoa appends the decimal form of v to buf without allocating.
func Itoa(buf []byte, v int) []byte {
	if v < 0 {
		buf = append(buf, '-')
		v = -v
	}
	var tmp [11]byte
	i := len(tmp)
	for {
		i--
		tmp[i] = byte('0' + v%10)
		v /= 10
		if v == 0 {
			break
		}
	}
	return append(buf, tmp[i:]...)
}

// String returns buf as a string without copying. buf must not be modified
// while the string is in use.
func String(buf []byte) string {
	if len(buf) == 0 {
		return ""
	}
	return unsafe.String(&buf[0], len(buf))
}
