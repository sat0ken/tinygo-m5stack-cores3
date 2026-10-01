// Package pngstream decodes a PNG image row by row into RGB565, so that a
// full-screen image can be drawn without holding the whole bitmap in memory
// (a 320x240 RGBA image would need 300 KB with image/png).
//
// Chunk CRCs are not verified (the image data is checked by zlib's Adler-32).
//
// Supported: bit depth 8, color types 0 (gray), 2 (RGB), 3 (palette),
// 4 (gray + alpha), 6 (RGBA); non-interlaced. Transparent pixels are blended
// over black.
//
// ref: PNG specification (W3C, Third Edition), 7.3 Filtering, 9 Filter types
package pngstream

import (
	"compress/zlib"
	"encoding/binary"
	"errors"
	"io"
)

var (
	ErrNotPNG      = errors.New("pngstream: not a PNG file")
	ErrUnsupported = errors.New("pngstream: unsupported PNG format")
	ErrFormat      = errors.New("pngstream: invalid PNG data")
)

// Header is the IHDR content.
type Header struct {
	Width, Height int
	BitDepth      uint8
	ColorType     uint8
	Interlace     uint8
}

// Decoder holds the buffers; reuse it for several images to avoid
// allocating per image. MaxWidth bounds the row buffers.
type Decoder struct {
	MaxWidth int

	cur, prev []byte // filtered rows (with bpp bytes of leading zero padding)
	out       []byte // RGB565 big endian row
	palette   [256][3]byte
	trns      [256]byte
	idat      idatReader
	chunkHdr  [8]byte
}

// RowFunc receives row y as big-endian RGB565 (2*Width bytes). The slice is
// reused for the next row.
type RowFunc func(y int, rgb565 []byte) error

var pngSig = [8]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}

// Decode reads a PNG from r and calls fn for each row from top to bottom.
func (d *Decoder) Decode(r io.Reader, fn RowFunc) (Header, error) {
	var h Header
	var sig [8]byte
	if _, err := io.ReadFull(r, sig[:]); err != nil || sig != pngSig {
		return h, ErrNotPNG
	}
	for i := range d.trns {
		d.trns[i] = 0xFF
	}
	// Read chunks up to the first IDAT.
	for {
		length, typ, err := d.readChunkHeader(r)
		if err != nil {
			return h, err
		}
		switch typ {
		case "IHDR":
			var b [13]byte
			if length != 13 {
				return h, ErrFormat
			}
			if _, err := io.ReadFull(r, b[:]); err != nil {
				return h, err
			}
			h.Width = int(binary.BigEndian.Uint32(b[0:]))
			h.Height = int(binary.BigEndian.Uint32(b[4:]))
			h.BitDepth, h.ColorType, h.Interlace = b[8], b[9], b[12]
			if h.BitDepth != 8 || h.Interlace != 0 || bpp(h.ColorType) == 0 {
				return h, ErrUnsupported
			}
			if h.Width <= 0 || h.Height <= 0 || (d.MaxWidth > 0 && h.Width > d.MaxWidth) {
				return h, ErrUnsupported
			}
			if err := skip(r, 4); err != nil { // CRC
				return h, err
			}
		case "PLTE":
			n := int(length / 3)
			for i := 0; i < n && i < 256; i++ {
				if _, err := io.ReadFull(r, d.palette[i][:]); err != nil {
					return h, err
				}
			}
			if err := skip(r, int64(length)-int64(n*3)+4); err != nil {
				return h, err
			}
		case "tRNS":
			if h.ColorType == 3 && length <= 256 {
				if _, err := io.ReadFull(r, d.trns[:length]); err != nil {
					return h, err
				}
				if err := skip(r, 4); err != nil {
					return h, err
				}
			} else if err := skip(r, int64(length)+4); err != nil {
				return h, err
			}
		case "IDAT":
			if h.Width == 0 {
				return h, ErrFormat
			}
			d.idat = idatReader{r: r, d: d, remain: length}
			return h, d.decodeRows(h, fn)
		case "IEND":
			return h, ErrFormat
		default:
			if err := skip(r, int64(length)+4); err != nil {
				return h, err
			}
		}
	}
}

func (d *Decoder) readChunkHeader(r io.Reader) (uint32, string, error) {
	if _, err := io.ReadFull(r, d.chunkHdr[:]); err != nil {
		return 0, "", err
	}
	return binary.BigEndian.Uint32(d.chunkHdr[:4]), string(d.chunkHdr[4:8]), nil
}

func (d *Decoder) decodeRows(h Header, fn RowFunc) error {
	pb := bpp(h.ColorType)
	stride := h.Width * pb
	if cap(d.cur) < stride+pb {
		d.cur = make([]byte, stride+pb)
		d.prev = make([]byte, stride+pb)
	}
	if cap(d.out) < h.Width*2 {
		d.out = make([]byte, h.Width*2)
	}
	cur, prev := d.cur[:stride+pb], d.prev[:stride+pb]
	for i := range prev {
		prev[i] = 0
		cur[i] = 0
	}
	out := d.out[:h.Width*2]

	zr, err := zlib.NewReader(&d.idat)
	if err != nil {
		return err
	}
	defer zr.Close()

	var ft [1]byte
	for y := 0; y < h.Height; y++ {
		if _, err := io.ReadFull(zr, ft[:]); err != nil {
			return err
		}
		if _, err := io.ReadFull(zr, cur[pb:]); err != nil {
			return err
		}
		if err := unfilter(ft[0], cur, prev, pb); err != nil {
			return err
		}
		d.toRGB565(h.ColorType, cur[pb:], out)
		if err := fn(y, out); err != nil {
			return err
		}
		cur, prev = prev, cur
	}
	return nil
}

// unfilter reverses the filter in place. cur and prev start with pb zero bytes
// so that "a" and "c" of the first pixel read as 0.
// ref: PNG specification 9.2 Filter types for filter method 0
func unfilter(ft byte, cur, prev []byte, pb int) error {
	n := len(cur)
	switch ft {
	case 0:
	case 1: // Sub
		for i := pb; i < n; i++ {
			cur[i] += cur[i-pb]
		}
	case 2: // Up
		for i := pb; i < n; i++ {
			cur[i] += prev[i]
		}
	case 3: // Average
		for i := pb; i < n; i++ {
			cur[i] += uint8((int(cur[i-pb]) + int(prev[i])) / 2)
		}
	case 4: // Paeth
		for i := pb; i < n; i++ {
			cur[i] += paeth(cur[i-pb], prev[i], prev[i-pb])
		}
	default:
		return ErrFormat
	}
	return nil
}

func paeth(a, b, c uint8) uint8 {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	if pa <= pb && pa <= pc {
		return a
	}
	if pb <= pc {
		return b
	}
	return c
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func bpp(colorType uint8) int {
	switch colorType {
	case 0, 3:
		return 1
	case 4:
		return 2
	case 2:
		return 3
	case 6:
		return 4
	}
	return 0
}

func (d *Decoder) toRGB565(ct uint8, src, out []byte) {
	w := len(out) / 2
	for x := 0; x < w; x++ {
		var r, g, b, a uint8 = 0, 0, 0, 0xFF
		switch ct {
		case 0:
			r = src[x]
			g, b = r, r
		case 2:
			r, g, b = src[x*3], src[x*3+1], src[x*3+2]
		case 3:
			p := src[x]
			r, g, b, a = d.palette[p][0], d.palette[p][1], d.palette[p][2], d.trns[p]
		case 4:
			r, a = src[x*2], src[x*2+1]
			g, b = r, r
		case 6:
			r, g, b, a = src[x*4], src[x*4+1], src[x*4+2], src[x*4+3]
		}
		if a != 0xFF { // blend over black
			r = uint8(uint16(r) * uint16(a) / 255)
			g = uint8(uint16(g) * uint16(a) / 255)
			b = uint8(uint16(b) * uint16(a) / 255)
		}
		v := uint16(r&0xF8)<<8 | uint16(g&0xFC)<<3 | uint16(b)>>3
		out[x*2], out[x*2+1] = byte(v>>8), byte(v)
	}
}

// idatReader concatenates the data of consecutive IDAT chunks.
type idatReader struct {
	r      io.Reader
	d      *Decoder
	remain uint32
}

func (ir *idatReader) Read(p []byte) (int, error) {
	for ir.remain == 0 {
		if err := skip(ir.r, 4); err != nil { // CRC of the previous chunk
			return 0, err
		}
		length, typ, err := ir.d.readChunkHeader(ir.r)
		if err != nil {
			return 0, err
		}
		if typ != "IDAT" {
			return 0, io.EOF
		}
		ir.remain = length
	}
	if uint32(len(p)) > ir.remain {
		p = p[:ir.remain]
	}
	n, err := ir.r.Read(p)
	ir.remain -= uint32(n)
	return n, err
}

func skip(r io.Reader, n int64) error {
	if n <= 0 {
		return nil
	}
	if s, ok := r.(io.Seeker); ok {
		_, err := s.Seek(n, io.SeekCurrent)
		return err
	}
	_, err := io.CopyN(io.Discard, r, n)
	return err
}
