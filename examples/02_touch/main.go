// 02_touch ports the M5Stack CoreS3 "Touch" Arduino tutorial: shows the touch
// state (touch / hold / flick / drag and their begin / end) and the coordinates.
//
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/touch
//
// The tutorial uses fonts::Orbitron_Light_24, which tinyfont does not have;
// FreeSans 12pt is used instead.
package main

import (
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/internal/gfx"

	"tinygo.org/x/tinyfont/freesans"
)

var font = &freesans.Regular12pt7b

func main() {
	time.Sleep(2 * time.Second) // give the serial monitor time to attach
	println("=== 02_touch ===")

	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	d, err := cores3.InitDisplay()
	if err != nil {
		println("InitDisplay failed:", err.Error())
	}
	if err := cores3.InitTouch(); err != nil {
		println("InitTouch failed:", err.Error())
	}
	cores3.Verbose = false

	w, h := d.Size()
	gfx.DrawTextCentered(d, font, w/2, h/2, "Touch Test", gfx.Green)

	prevX, prevY := int16(-1), int16(-1)
	prevState := cores3.TouchNone
	var text [32]byte
	start := time.Now()
	for {
		ms := uint32(time.Since(start).Milliseconds())
		cores3.Touch.Update(ms)

		t := cores3.Touch.Detail()
		if prevState != t.State {
			prevState = t.State
			println(t.State.String())
			d.FillRectangle565(0, 0, w, 140, gfx.Black)
			gfx.DrawTextCentered(d, font, w/2, h/2-30, t.State.String(), gfx.Green)
		}
		if prevX != t.X || prevY != t.Y {
			d.FillRectangle565(0, 140, w, 100, gfx.Black)
			s := append(text[:0], "X:"...)
			s = gfx.Itoa(s, int(t.X))
			s = append(s, " / Y:"...)
			s = gfx.Itoa(s, int(t.Y))
			gfx.DrawTextCentered(d, font, w/2, 200, gfx.String(s), gfx.Green)
			println(gfx.String(s))
			prevX, prevY = t.X, t.Y
			gfx.FillCircle(d, prevX, prevY, 4, gfx.Green)
		}
		time.Sleep(time.Millisecond)
	}
}
