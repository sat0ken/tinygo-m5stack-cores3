// 08_wakeup ports the M5Stack CoreS3 "Wakeup" Arduino tutorial: touch the
// screen to power off; the RTC timer powers the board on again after 5 s.
//
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/wakeup
//
//	if (M5.Touch.getCount() && M5.Touch.getDetail(0).wasClicked()) M5.Power.timerSleep(5);
package main

import (
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/drivers/bm8563"
	"esp32-s3-lite/internal/gfx"

	"tinygo.org/x/tinyfont/freeserif"
)

var font = &freeserif.Italic18pt7b

func main() {
	time.Sleep(2 * time.Second) // give the serial monitor time to attach
	println("=== 08_wakeup ===")

	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	d, err := cores3.InitDisplay()
	if err != nil {
		println("InitDisplay failed:", err.Error())
	}
	cores3.Verbose = false
	if err := cores3.InitTouch(); err != nil {
		println("InitTouch failed:", err.Error())
	}
	if err := cores3.InitRTC(); err != nil {
		println("InitRTC failed:", err.Error())
	}
	// Log whether this boot was caused by the RTC timer (TF still set).
	ctrl2, _ := cores3.RTC.ReadRegister(bm8563.RegControl2)
	println("RTC CTRL2 =", ctrl2, " timer flag (woken by RTC):", ctrl2&bm8563.Ctrl2TF != 0)
	cores3.RTC.ClearIRQ()

	w, h := d.Size()
	gfx.DrawTextCentered(d, font, w/2, h/2-60, "Touch", gfx.Green)
	gfx.DrawTextCentered(d, font, w/2, h/2-20, "to sleep", gfx.Green)
	gfx.DrawTextCentered(d, font, w/2, h/2+20, "After 5s", gfx.Green)
	gfx.DrawTextCentered(d, font, w/2, h/2+60, "Wakeup", gfx.Green)

	start := time.Now()
	for {
		ms := uint32(time.Since(start).Milliseconds())
		cores3.Touch.Update(ms)
		if cores3.Touch.Count() > 0 && cores3.Touch.Detail().WasClicked() {
			println("touched: power off, RTC wakes up in 5 s")
			time.Sleep(50 * time.Millisecond) // let the log line out
			if err := cores3.TimerSleep(5 * time.Second); err != nil {
				println("TimerSleep failed:", err.Error())
			}
		}
		time.Sleep(time.Millisecond)
	}
}
