// 01_display ports the M5Stack CoreS3 "Display" Arduino tutorial:
// filled circles and squares at random positions in random colors, forever.
//
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/display
//
//	r = (width >> 4) + 2; fillCircle(x, y, r, rand()); fillRect(x-r, y-r, 2r, 2r, rand())
package main

import (
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/drivers/ili9342"
)

var seed uint32 = 0x12345678

// xorshift32: rand() replacement without heap allocation.
func rand() uint32 {
	seed ^= seed << 13
	seed ^= seed >> 17
	seed ^= seed << 5
	return seed
}

func main() {
	time.Sleep(2 * time.Second) // give `tinygo monitor` time to attach
	println("=== 01_display ===")

	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	d, err := cores3.InitDisplay()
	if err != nil {
		println("InitDisplay failed:", err.Error())
		for {
			time.Sleep(time.Second)
		}
	}
	w, h := d.Size()
	println("display:", cores3.LCDProbe.Variant.String(), w, "x", h)

	// Color / orientation check pattern, shown for 3 seconds:
	// red, green, blue bars from left to right, white square at top-left.
	d.FillRectangle565(0, 0, w/3, h, 0xF800)
	d.FillRectangle565(w/3, 0, w/3, h, 0x07E0)
	d.FillRectangle565(2*w/3, 0, w-2*w/3, h, 0x001F)
	d.FillRectangle565(0, 0, 40, 40, 0xFFFF)
	println("check pattern: R | G | B bars, white square top-left")
	time.Sleep(3 * time.Second)

	r := (w >> 4) + 2
	count := 0
	start := time.Now()
	for {
		x := int16(rand() % uint32(w))
		y := int16(rand() % uint32(h))
		fillCircle(d, x, y, r, uint16(rand()))

		x = int16(rand() % uint32(w))
		y = int16(rand() % uint32(h))
		d.FillRectangle565(x-r, y-r, r*2, r*2, uint16(rand()))

		count++
		if count == 500 {
			ms := time.Since(start).Milliseconds()
			println("500 circle+rect pairs in", ms, "ms")
			count = 0
			start = time.Now()
		}
	}
}

// fillCircle draws a filled circle as horizontal spans (clipped by the driver).
func fillCircle(d *ili9342.Device, x0, y0, r int16, c uint16) {
	rr := int32(r) * int32(r)
	dx := r
	for dy := int16(0); dy <= r; dy++ {
		for dx > 0 && int32(dx)*int32(dx)+int32(dy)*int32(dy) > rr {
			dx--
		}
		d.FillRectangle565(x0-dx, y0+dy, 2*dx+1, 1, c)
		if dy != 0 {
			d.FillRectangle565(x0-dx, y0-dy, 2*dx+1, 1, c)
		}
	}
}
