// Package bmp writes 24 bit uncompressed Windows BMP files from big-endian
// RGB565 pixel data, one row at a time.
//
// ref: Microsoft BITMAPFILEHEADER / BITMAPINFOHEADER (BI_RGB, bottom-up rows,
// rows padded to 4 bytes)
package bmp

import (
	"encoding/binary"
	"io"
)

// Size returns the file size of a w x h 24 bit BMP.
func Size(w, h int) int {
	return 54 + rowSize(w)*h
}

func rowSize(w int) int { return (w*3 + 3) &^ 3 }

// WriteRGB565BE writes pix (w*h pixels, 2 bytes each, high byte first, top
// row first) as a 24 bit BMP. row must have room for one padded BMP row
// (w*3 rounded up to 4 bytes).
func WriteRGB565BE(out io.Writer, pix []byte, w, h int, row []byte) error {
	var hdr [54]byte
	rs := rowSize(w)
	copy(hdr[0:], "BM")
	binary.LittleEndian.PutUint32(hdr[2:], uint32(Size(w, h)))
	binary.LittleEndian.PutUint32(hdr[10:], 54) // pixel data offset
	binary.LittleEndian.PutUint32(hdr[14:], 40) // BITMAPINFOHEADER
	binary.LittleEndian.PutUint32(hdr[18:], uint32(w))
	binary.LittleEndian.PutUint32(hdr[22:], uint32(h)) // positive: bottom-up
	binary.LittleEndian.PutUint16(hdr[26:], 1)         // planes
	binary.LittleEndian.PutUint16(hdr[28:], 24)        // bits per pixel
	binary.LittleEndian.PutUint32(hdr[34:], uint32(rs*h))
	binary.LittleEndian.PutUint32(hdr[38:], 2835) // 72 dpi
	binary.LittleEndian.PutUint32(hdr[42:], 2835)
	if _, err := out.Write(hdr[:]); err != nil {
		return err
	}
	row = row[:rs]
	for i := w * 3; i < rs; i++ {
		row[i] = 0
	}
	for y := h - 1; y >= 0; y-- {
		src := pix[y*w*2:]
		for x := 0; x < w; x++ {
			v := uint16(src[x*2])<<8 | uint16(src[x*2+1])
			r := uint8(v>>11) & 31
			g := uint8(v>>5) & 63
			b := uint8(v) & 31
			row[x*3] = b<<3 | b>>2
			row[x*3+1] = g<<2 | g>>4
			row[x*3+2] = r<<3 | r>>2
		}
		if _, err := out.Write(row); err != nil {
			return err
		}
	}
	return nil
}
