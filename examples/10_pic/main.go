// 10_pic ports the M5Stack CoreS3 "Image Files" Arduino tutorial: the same
// steps as the microSD tutorial, then /TestPicture01.png and
// /TestPicture02.png alternately every second.
//
// ref: https://docs.m5stack.com/en/arduino/m5cores3/pic
//
// The tutorial uses fonts::Orbitron_Light_24, which tinyfont does not have;
// FreeSans 12pt is used instead. Put the images on the card first
// (tools/sdput).
package main

import (
	"os"
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/internal/gfx"

	"tinygo.org/x/tinyfont/freesans"
)

func main() {
	time.Sleep(2 * time.Second) // give the serial monitor time to attach
	println("=== 10_pic ===")

	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	d, err := cores3.InitDisplay()
	if err != nil {
		println("InitDisplay failed:", err.Error())
	}
	cores3.Verbose = false
	con := gfx.Console{D: d, Font: &freesans.Regular12pt7b, Fore: gfx.White, Back: gfx.Black}
	print1 := func(s string) {
		print(s)
		con.Print(s)
	}

	if err := cores3.InitSD(); err != nil {
		println("InitSD:", err.Error())
		print1("\n SD card not detected\n")
		for {
			time.Sleep(time.Second)
		}
	}
	print1("\n SD card detected\n")
	time.Sleep(time.Second)

	print1("\n SD card write test...\n")
	f, err := cores3.FS.OpenFile("/WriteTest.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err == nil {
		_, err = f.Write([]byte("Hello, world! \nSD card write success! \n"))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err == nil {
		print1(" SD card write success\n")
	} else {
		print1(" Failed to create TXT file\n")
	}
	time.Sleep(time.Second)

	print1("\n SD card read test...\n")
	if _, err := cores3.FS.Stat("/TestPicture01.png"); err == nil {
		print1(" PNG file 01 detected\n")
	} else {
		print1(" PNG file 01 not detected\n")
	}
	if _, err := cores3.FS.Stat("/TestPicture02.png"); err == nil {
		print1(" PNG file 02 detected\n")
	} else {
		print1(" PNG file 02 not detected\n")
	}

	for {
		if err := cores3.DrawPNGFile("/TestPicture01.png", 0, 0); err != nil {
			println("drawPngFile failed:", err.Error())
		}
		time.Sleep(time.Second)
		if err := cores3.DrawPNGFile("/TestPicture02.png", 0, 0); err != nil {
			println("drawPngFile failed:", err.Error())
		}
		time.Sleep(time.Second)
	}
}
