package main

import (
	"encoding/binary"
	"math"
	"os"

	"esp32-s3-lite/cores3"
)

// The generated test sound (12 s, 44.1 kHz, 16 bit, stereo, about -12 dBFS):
//
//	0-4 s : C-E-G-C arpeggio twice, alternating LEFT only / RIGHT only
//	4-12 s: C major scale up and down, both channels
const (
	wavRate    = 44100
	wavSeconds = 12
)

var wavBuf [4096]byte

func putGeneratedWAV(path string) {
	if fi, err := cores3.FS.Stat(path); err == nil {
		println("skip (exists):", path, fi.Size(), "bytes")
		return
	}
	f, err := cores3.FS.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		println("create failed:", path, err.Error())
		return
	}
	total := wavRate * wavSeconds
	dataSize := uint32(total * 4)
	h := wavBuf[:44]
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], 36+dataSize)
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], 2)
	binary.LittleEndian.PutUint32(h[24:], wavRate)
	binary.LittleEndian.PutUint32(h[28:], wavRate*4)
	binary.LittleEndian.PutUint16(h[32:], 4)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], dataSize)
	if _, err := f.Write(h); err != nil {
		println("write failed:", err.Error())
		f.Close()
		return
	}

	const c, e, g, c2 = 261.63, 329.63, 392.00, 523.25
	scale := [8]float64{261.63, 293.66, 329.63, 349.23, 392.00, 440.00, 493.88, 523.25}
	var notes [24]float64
	var pans [24]float64
	arp := [4]float64{c, e, g, c2}
	for k := 0; k < 8; k++ {
		notes[k] = arp[k%4]
		if k%2 == 0 {
			pans[k] = -1 // left only
		} else {
			pans[k] = 1 // right only
		}
	}
	for k := 0; k < 8; k++ {
		notes[8+k] = scale[k]
		notes[16+k] = scale[7-k]
	}

	written := 0
	pos := 0
	noteLen := wavRate / 2
	for k := 0; k < len(notes) && written < total; k++ {
		for i := 0; i < noteLen && written < total; i++ {
			t := float64(i) / wavRate
			env := math.Min(1, t/0.01) * math.Exp(-3*t)
			v := env * (math.Sin(2*math.Pi*notes[k]*t) + 0.3*math.Sin(4*math.Pi*notes[k]*t)) * 0.25 / 1.3
			l, r := v, v
			if pans[k] < 0 {
				r = 0
			} else if pans[k] > 0 {
				l = 0
			}
			binary.LittleEndian.PutUint16(wavBuf[pos:], uint16(int16(l*32767)))
			binary.LittleEndian.PutUint16(wavBuf[pos+2:], uint16(int16(r*32767)))
			pos += 4
			written++
			if pos == len(wavBuf) {
				if _, err := f.Write(wavBuf[:]); err != nil {
					println("write failed:", err.Error())
					f.Close()
					return
				}
				pos = 0
			}
		}
	}
	if pos > 0 {
		f.Write(wavBuf[:pos])
	}
	if err := f.Close(); err != nil {
		println("close failed:", err.Error())
		return
	}
	fi, err := cores3.FS.Stat(path)
	if err != nil || fi.Size() != int64(44+dataSize) {
		println("verify failed:", path)
		return
	}
	println("wrote:", path, fi.Size(), "bytes")
}
