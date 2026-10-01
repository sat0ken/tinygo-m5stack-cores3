// 11_speaker ports the M5Stack CoreS3 "Speaker" Arduino tutorial: a 10 kHz
// tone for 100 ms and a 4 kHz tone for 20 ms, one second apart.
//
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/speaker
//
// M5.Speaker.tone() returns at once and the loop then waits 1 s; here Tone
// returns after the tone, so the waits are shortened by the tone length.
package main

import (
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/internal/gfx"

	"tinygo.org/x/tinyfont/freesans"
)

func main() {
	time.Sleep(2 * time.Second) // give the serial monitor time to attach
	println("=== 11_speaker ===")

	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	d, err := cores3.InitDisplay()
	if err != nil {
		println("InitDisplay failed:", err.Error())
	}
	cores3.Verbose = false
	w, h := d.Size()
	gfx.DrawTextCentered(d, &freesans.Regular12pt7b, w/2, h/2, "Speaker Test", gfx.Green)

	if err := cores3.InitSpeaker(); err != nil {
		println("InitSpeaker failed:", err.Error())
	}
	for _, r := range []uint8{0x04, 0x05, 0x06, 0x0C, 0x61} {
		v, err := cores3.Amp.ReadRegister(r)
		print("AW88298 reg 0x", r, " = ", v)
		if err != nil {
			print(" err ", err.Error())
		}
		println()
	}

	for {
		start := time.Now()
		cores3.Tone(10000, 100)
		println("tone 10kHz 100ms took", time.Since(start).Milliseconds(), "ms")
		time.Sleep(900 * time.Millisecond)
		start = time.Now()
		cores3.Tone(4000, 20)
		println("tone 4kHz 20ms took", time.Since(start).Milliseconds(), "ms")
		time.Sleep(980 * time.Millisecond)
	}
}
