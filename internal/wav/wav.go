// Package wav parses the header of a RIFF/WAVE PCM file and leaves the reader
// at the start of the sample data.
//
// ref: Microsoft "Multimedia Programming Interface and Data Specifications 1.0" (RIFF, WAVE fmt / data chunks)
package wav

import (
	"encoding/binary"
	"errors"
	"io"
)

var (
	ErrNotWAV      = errors.New("wav: not a RIFF/WAVE file")
	ErrUnsupported = errors.New("wav: unsupported format (PCM 8/16 bit, 1/2 channels only)")
)

// Format is the "fmt " chunk content plus the data size.
type Format struct {
	SampleRate    uint32
	Channels      uint16
	BitsPerSample uint16
	DataSize      uint32 // bytes of sample data
}

// ReadHeader reads chunks up to "data".
func ReadHeader(r io.Reader) (Format, error) {
	var f Format
	var hdr [12]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return f, err
	}
	if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WAVE" {
		return f, ErrNotWAV
	}
	gotFmt := false
	var ch [8]byte
	for {
		if _, err := io.ReadFull(r, ch[:]); err != nil {
			return f, err
		}
		size := binary.LittleEndian.Uint32(ch[4:])
		switch string(ch[0:4]) {
		case "fmt ":
			var b [16]byte
			if size < 16 {
				return f, ErrNotWAV
			}
			if _, err := io.ReadFull(r, b[:]); err != nil {
				return f, err
			}
			if binary.LittleEndian.Uint16(b[0:]) != 1 { // WAVE_FORMAT_PCM
				return f, ErrUnsupported
			}
			f.Channels = binary.LittleEndian.Uint16(b[2:])
			f.SampleRate = binary.LittleEndian.Uint32(b[4:])
			f.BitsPerSample = binary.LittleEndian.Uint16(b[14:])
			if err := skip(r, int64(size-16)+int64(size&1)); err != nil {
				return f, err
			}
			gotFmt = true
		case "data":
			if !gotFmt {
				return f, ErrNotWAV
			}
			if (f.BitsPerSample != 8 && f.BitsPerSample != 16) || f.Channels < 1 || f.Channels > 2 {
				return f, ErrUnsupported
			}
			f.DataSize = size
			return f, nil
		default:
			if err := skip(r, int64(size)+int64(size&1)); err != nil { // chunks are word aligned
				return f, err
			}
		}
	}
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
