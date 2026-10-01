// 09_sdcard ports the M5Stack CoreS3 "microSD" Arduino tutorial: detect the
// card, write /WriteTest.txt, check the two test PNGs and show them
// alternately every 500 ms.
//
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/sdcard
//
// Put the test images on the card first (tools/sdput).
package main

import (
	"os"
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/internal/gfx"

	"tinygo.org/x/tinyfont/freemono"
)

func main() {
	time.Sleep(2 * time.Second) // give the serial monitor time to attach
	println("=== 09_sdcard ===")

	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	d, err := cores3.InitDisplay()
	if err != nil {
		println("InitDisplay failed:", err.Error())
	}
	cores3.Verbose = false
	con := gfx.Console{D: d, Font: &freemono.Regular12pt7b, Fore: gfx.White, Back: gfx.Black}
	say := func(s string) {
		println(s)
		con.Print(s)
		con.Print("\n")
	}

	if err := cores3.InitSD(); err != nil {
		println("InitSD:", err.Error())
		say("SD card not detected")
		for {
			time.Sleep(time.Second)
		}
	}
	say("SD card detected")
	time.Sleep(time.Second)

	say("SD card write test...")
	f, err := cores3.FS.OpenFile("/WriteTest.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err == nil {
		_, err = f.Write([]byte("Hello, world! \nSD card write success! \n"))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err == nil {
		say("SD card write success")
	} else {
		println(err.Error())
		say("Failed to create TXT file")
	}
	time.Sleep(time.Second)

	say("SD card read test...")
	if _, err := cores3.FS.Stat("/cores3_test_picture01.png"); err == nil {
		say("PNG file 01 detected")
	} else {
		say("PNG file 01 not detected")
	}
	if _, err := cores3.FS.Stat("/cores3_test_picture02.png"); err == nil {
		say("PNG file 02 detected")
	} else {
		say("PNG file 02 not detected")
	}
	time.Sleep(time.Second)

	for {
		draw("/cores3_test_picture01.png")
		time.Sleep(500 * time.Millisecond)
		draw("/cores3_test_picture02.png")
		time.Sleep(500 * time.Millisecond)
	}
}

func draw(path string) {
	start := time.Now()
	if err := cores3.DrawPNGFile(path, 0, 0); err != nil {
		println("drawPngFile", path, "failed:", err.Error())
		return
	}
	println(path, "drawn in", time.Since(start).Milliseconds(), "ms")
}
