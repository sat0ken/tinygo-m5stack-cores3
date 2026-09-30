// 00_power_on runs the CoreS3 power-up sequence (M1) and dumps the AXP2101 /
// AW9523B registers.
//
// Expected: the LCD backlight turns on (the panel glows, the image may be
// garbage or black because the LCD controller is not initialized yet), and the
// serial log shows each register write with its read-back value.
package main

import (
	"time"

	"esp32-s3-lite/cores3"
)

func main() {
	time.Sleep(2 * time.Second) // give `tinygo monitor` time to attach

	println("=== CoreS3 power-on (M1) ===")
	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	cores3.DumpRegisters()

	// Blink the backlight between two levels so it is obvious that DLDO1 works.
	for {
		time.Sleep(2 * time.Second)
		cores3.Verbose = false
		cores3.SetBrightness(255)
		println("backlight 255")
		time.Sleep(2 * time.Second)
		cores3.SetBrightness(64)
		println("backlight 64")
	}
}
