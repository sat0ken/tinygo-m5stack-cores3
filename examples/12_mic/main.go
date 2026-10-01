// 12_mic ports the M5Stack CoreS3 "MIC" Arduino tutorial: record at 17 kHz
// into a ring of 256 x 320 samples while drawing the waveform; touch the
// screen to play the last ~4.8 s back; the power button changes the noise
// filter level.
//
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/mic
//
// Differences: the tutorial's Speaker.setVolume(255) has no equivalent (PCM is
// played as recorded). As in M5Unified, playback runs at 48 kHz and the 17 kHz
// recording is resampled.
package main

import (
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/internal/gfx"

	"tinygo.org/x/tinyfont/freesans"
)

const (
	recordNumber     = 256
	recordLength     = 320
	recordSize       = recordNumber * recordLength
	recordSamplerate = 17000
)

var (
	recData       [recordSize]int16
	prevY         [recordLength]int16
	prevH         [recordLength]int16
	recRecordIdx  = 2
	drawRecordIdx = 0
	font          = &freesans.BoldOblique12pt7b
)

const playColor = 0x1c9f // as in the tutorial

func main() {
	time.Sleep(2 * time.Second) // give the serial monitor time to attach
	println("=== 12_mic ===")

	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	d, err := cores3.InitDisplay()
	if err != nil {
		println("InitDisplay failed:", err.Error())
	}
	cores3.Verbose = false
	cores3.InitTouch()

	if err := cores3.StartMic(recordSamplerate); err != nil {
		println("StartMic failed:", err.Error())
	}
	drawREC(d)

	w, h := d.Size()
	if w > recordLength-1 {
		w = recordLength - 1
	}
	const shift = 6
	start := time.Now()
	frames := 0
	for {
		ms := uint32(time.Since(start).Milliseconds())
		cores3.Touch.Update(ms)
		cores3.BtnPWR.Update(ms)

		cores3.ReadMic(recData[recRecordIdx*recordLength : (recRecordIdx+1)*recordLength])
		data := recData[drawRecordIdx*recordLength : (drawRecordIdx+1)*recordLength]
		for x := int16(0); x < w; x++ {
			d.FillRectangle565(x, prevY[x], 1, prevH[x], gfx.Black)
			y1 := int16(data[x] >> shift)
			y2 := int16(data[x+1] >> shift)
			if y1 > y2 {
				y1, y2 = y2, y1
			}
			y := h>>1 + y1
			hh := h>>1 + y2 + 1 - y
			prevY[x], prevH[x] = y, hh
			d.FillRectangle565(x, y, 1, hh, gfx.White)
		}
		drawREC(d)
		if drawRecordIdx++; drawRecordIdx >= recordNumber {
			drawRecordIdx = 0
		}
		if recRecordIdx++; recRecordIdx >= recordNumber {
			recRecordIdx = 0
		}
		if frames++; frames%100 == 0 {
			println("frames", frames, "peak", peak(data))
		}

		if cores3.BtnPWR.WasClicked() {
			cores3.MicNoiseFilterLevel += 8
			println("noise filter level", cores3.MicNoiseFilterLevel)
			clearScreen(d)
			gfx.FillCircle(d, 130, 15, 8, gfx.Green)
			var b [16]byte
			s := gfx.Itoa(append(b[:0], "NF:"...), int(cores3.MicNoiseFilterLevel))
			gfx.DrawTextTopCenter(d, font, 180, 3, gfx.String(s), gfx.White)
			time.Sleep(2 * time.Second)
			clearScreen(d)
		}

		if cores3.Touch.Count() > 0 && cores3.Touch.Detail().WasClicked() {
			clearScreen(d)
			cores3.StopMic()
			if err := cores3.StartSpeaker(cores3.SpeakerSampleRate); err != nil {
				println("StartSpeaker failed:", err.Error())
			}
			gfx.FillTriangle(d, 130-8, 15-8, 130-8, 15+8, 130+8, 15, playColor)
			gfx.DrawTextTopCenter(d, font, 180, 3, "PLAY", gfx.White)
			println("play from block", recRecordIdx)
			startPos := recRecordIdx * recordLength
			cores3.QueuePCMResampled(recData[startPos:], recordSamplerate)
			if startPos > 0 {
				cores3.QueuePCMResampled(recData[:startPos], recordSamplerate)
			}
			cores3.FlushResampled()
			cores3.StopSpeaker()
			if err := cores3.StartMic(recordSamplerate); err != nil {
				println("StartMic failed:", err.Error())
			}
			clearScreen(d)
			drawREC(d)
			prevY, prevH = [recordLength]int16{}, [recordLength]int16{}
		}
	}
}

func drawREC(d gfx.Filler) {
	gfx.FillCircle(d, 130, 15, 8, gfx.Red)
	gfx.DrawTextTopCenter(d, font, 180, 3, "REC", gfx.White)
}

func peak(s []int16) int {
	m := 0
	for _, v := range s {
		a := int(v)
		if a < 0 {
			a = -a
		}
		if a > m {
			m = a
		}
	}
	return m
}

func clearScreen(d gfx.Filler) {
	w, h := d.Size()
	d.FillRectangle565(0, 0, w, h, gfx.Black)
}
