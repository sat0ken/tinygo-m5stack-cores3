// sdput copies the tutorial test images onto the microSD card from the
// device itself, so the card does not have to be taken out.
//
//	./tools/sdput/fetch.sh
//	tinygo flash -target=esp32s3-generic ./tools/sdput
//
// Files that already exist on the card are never overwritten.
//
// The "Image Files" tutorial reads /TestPicture01.png and /TestPicture02.png,
// for which M5Stack publishes no files; the microSD tutorial images are
// written under those names as well. The "Audio Files" tutorial plays
// /sample-12s.wav, which is not published either; a 12 s test sound is
// generated on the device and written under that name (see wavgen.go).
//
// Note: the images are embedded as []byte (copied to RAM at start-up). Large
// embedded data (the 2 MB WAV) cannot be used: the ROM loader rejects the
// image ("Invalid image block, can't boot").
package main

import (
	_ "embed"
	"os"
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/drivers/bm8563"
)

//go:embed assets/cores3_test_picture01.png
var pic01 []byte

//go:embed assets/cores3_test_picture02.png
var pic02 []byte

func main() {
	time.Sleep(2 * time.Second) // give the serial monitor time to attach
	println("=== sdput ===")
	cores3.Init()
	cores3.InitDisplay()
	cores3.Verbose = false
	if err := cores3.InitSD(); err != nil {
		println("InitSD failed:", err.Error())
		// A full power cycle resets a card stuck in initialization. Do it once:
		// after the RTC wake-up the timer flag is set and we stop here.
		cores3.InitRTC()
		ctrl2, _ := cores3.RTC.ReadRegister(bm8563.RegControl2)
		if ctrl2&bm8563.Ctrl2TF == 0 {
			println("power-cycling the board (back in 3 s)")
			time.Sleep(100 * time.Millisecond)
			cores3.TimerSleep(3 * time.Second)
		}
		cores3.RTC.ClearIRQ()
		halt()
	}
	put("/cores3_test_picture01.png", pic01)
	put("/cores3_test_picture02.png", pic02)
	put("/TestPicture01.png", pic01)
	put("/TestPicture02.png", pic02)
	putGeneratedWAV("/sample-12s.wav")
	println("done")
	halt()
}

func put(path string, data []byte) {
	if fi, err := cores3.FS.Stat(path); err == nil {
		println("skip (exists):", path, fi.Size(), "bytes")
		return
	}
	f, err := cores3.FS.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		println("create failed:", path, err.Error())
		return
	}
	n, err := f.Write(data)
	cerr := f.Close()
	if err != nil || cerr != nil || n != len(data) {
		println("write failed:", path, n)
		return
	}
	fi, err := cores3.FS.Stat(path)
	if err != nil || fi.Size() != int64(len(data)) {
		println("verify failed:", path)
		return
	}
	println("wrote:", path, n, "bytes")
}

func halt() {
	for {
		time.Sleep(time.Second)
	}
}
