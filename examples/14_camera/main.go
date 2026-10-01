// 14_camera takes a photo with the GC0308 camera when the screen is touched
// and shows it (the first one is taken automatically after start-up). (The M5Stack tutorial streams video with frame buffers in
// PSRAM, which TinyGo does not support; see docs/notes.md.)
//
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/camera
package main

import (
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/internal/gfx"

	"tinygo.org/x/tinyfont/freesans"
)

var photo [cores3.PhotoWidth * cores3.PhotoHeight * 2]byte

func main() {
	time.Sleep(2 * time.Second) // give the serial monitor time to attach
	println("=== 14_camera ===")

	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	d, err := cores3.InitDisplay()
	if err != nil {
		println("InitDisplay failed:", err.Error())
	}
	cores3.Verbose = false
	cores3.InitTouch()
	w, h := d.Size()
	font := &freesans.Regular12pt7b

	if err := cores3.InitCamera(); err != nil {
		println("InitCamera failed:", err.Error())
		gfx.DrawTextCentered(d, font, w/2, h/2, "Camera Init Fail", gfx.Red)
		for {
			time.Sleep(time.Second)
		}
	}
	println("Camera Init Success")
	gfx.DrawTextCentered(d, font, w/2, h/2, "Touch to take a photo", gfx.White)

	time.Sleep(time.Second)
	start := time.Now()
	count := 0
	first := true // the first photo is taken without a touch
	for {
		cores3.Touch.Update(uint32(time.Since(start).Milliseconds()))
		if first || (cores3.Touch.Count() > 0 && cores3.Touch.Detail().WasClicked()) {
			first = false
			t0 := time.Now()
			if err := cores3.CapturePhoto(photo[:]); err != nil {
				println("capture failed:", err.Error())
				gfx.DrawTextCentered(d, font, w/2, 20, "Capture failed", gfx.Red)
				continue
			}
			t1 := time.Now()
			d.DrawRGBBitmap8(0, 0, photo[:], cores3.PhotoWidth, cores3.PhotoHeight)
			count++
			println("photo", count, "captured in", t1.Sub(t0).Milliseconds(), "ms, drawn in", time.Since(t1).Milliseconds(), "ms; first bytes", photo[0], photo[1], photo[2], photo[3])
		}
		time.Sleep(time.Millisecond)
	}
}
