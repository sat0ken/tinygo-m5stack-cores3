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

// Console prints text like M5GFX print/printf with a GFX font: the cursor is
// the top-left of the line, text wraps at the right edge, and each character
// cell is filled with Back before the glyph is drawn (M5GFX default text style:
// white on black, setTextWrap(true)).
type Console struct {
	D          Filler
	Font       *tinyfont.Font
	Fore, Back uint16
	X, Y       int16 // cursor (top-left of the current line)
}

// SetCursor moves the cursor.
func (c *Console) SetCursor(x, y int16) { c.X, c.Y = x, y }

// Print draws str at the cursor and advances it. '\n' starts a new line.
func (c *Console) Print(str string) {
	w, _ := c.D.Size()
	lineH := int16(c.Font.YAdvance)
	for _, r := range str {
		if r == '\n' {
			c.X = 0
			c.Y += lineH
			continue
		}
		if r == '\r' { // ignored, as in M5GFX
			continue
		}
		g := glyph(c.Font, r)
		adv := int16(g.XAdvance)
		if c.X+adv > w {
			c.X = 0
			c.Y += lineH
		}
		c.D.FillRectangle565(c.X, c.Y, adv, lineH, c.Back)
		drawGlyph(c.D, g, c.X, c.Y-int16(c.Font.BBox[3]), c.Fore)
		c.X += adv
	}
}

// Append2 appends v as two decimal digits (zero padded).
func Append2(buf []byte, v int) []byte {
	return append(buf, byte('0'+v/10%10), byte('0'+v%10))
}

// Append4 appends v as four decimal digits (zero padded).
func Append4(buf []byte, v int) []byte {
	return append(buf, byte('0'+v/1000%10), byte('0'+v/100%10), byte('0'+v/10%10), byte('0'+v%10))
}

// AppendFloat appends v with 6 fraction digits, like printf("%f").
func AppendFloat(buf []byte, v float32) []byte {
	f := float64(v)
	if f < 0 {
		buf = append(buf, '-')
		f = -f
	}
	n := int64(f*1e6 + 0.5)
	buf = Itoa(buf, int(n/1000000))
	buf = append(buf, '.')
	frac := int(n % 1000000)
	for d := 100000; d > 0; d /= 10 {
		buf = append(buf, byte('0'+frac/d%10))
	}
	return buf
}

// DrawTextTopCenter draws str horizontally centered on x with the top of the
// font bounding box at y (M5GFX setTextDatum(top_center)).
func DrawTextTopCenter(d Filler, font *tinyfont.Font, x, y int16, str string, c uint16) {
	w := TextWidth(font, str)
	DrawText(d, font, x-w/2, y-int16(font.BBox[3]), str, c)
}

// FillTriangle fills a triangle with horizontal spans.
func FillTriangle(d Filler, x0, y0, x1, y1, x2, y2 int16, c uint16) {
	// sort by y
	if y0 > y1 {
		x0, y0, x1, y1 = x1, y1, x0, y0
	}
	if y1 > y2 {
		x1, y1, x2, y2 = x2, y2, x1, y1
	}
	if y0 > y1 {
		x0, y0, x1, y1 = x1, y1, x0, y0
	}
	edge := func(xa, ya, xb, yb, y int16) int16 {
		if yb == ya {
			return xa
		}
		return xa + int16(int32(xb-xa)*int32(y-ya)/int32(yb-ya))
	}
	for y := y0; y <= y2; y++ {
		a := edge(x0, y0, x2, y2, y)
		var b int16
		if y < y1 {
			b = edge(x0, y0, x1, y1, y)
		} else {
			b = edge(x1, y1, x2, y2, y)
		}
		if a > b {
			a, b = b, a
		}
		d.FillRectangle565(a, y, b-a+1, 1, c)
	}
}
