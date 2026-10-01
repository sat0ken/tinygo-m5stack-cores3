// 03_button ports the M5Stack CoreS3 "Button" Arduino tutorial: shows
// "Btn PWR" when the power button is clicked.
//
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/button
//
// The power button is read through the AXP2101 (short press IRQ). A long press
// of about 4 s powers the board off (AXP2101 reg 0x27 = 0x00, set by Init).
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
	println("=== 03_button ===")

	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	d, err := cores3.InitDisplay()
	if err != nil {
		println("InitDisplay failed:", err.Error())
	}
	cores3.Verbose = false
	cores3.PMIC.ClearIRQStatus() // drop presses from before boot

	w, h := d.Size()
	gfx.DrawTextCentered(d, font, w/2, 15, "Touch Button Test", gfx.Red)

	clicks := 0
	start := time.Now()
	for {
		ms := uint32(time.Since(start).Milliseconds())
		cores3.BtnPWR.Update(ms)

		if cores3.BtnPWR.WasClicked() {
			clicks++
			println("BtnPWR clicked", clicks)
			d.FillRectangle565(0, 40, w, 70, gfx.Black)
			gfx.DrawTextCentered(d, font, w/2, h/2-30, "Btn PWR", gfx.Red)
		}
		if cores3.BtnPWR.WasHold() {
			println("BtnPWR hold")
		}
		time.Sleep(time.Millisecond)
	}
}
