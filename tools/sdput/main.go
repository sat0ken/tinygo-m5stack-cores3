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
// written under those names as well.
package main

import (
	_ "embed"
	"os"
	"time"

	"esp32-s3-lite/cores3"
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
		halt()
	}
	put("/cores3_test_picture01.png", pic01)
	put("/cores3_test_picture02.png", pic02)
	put("/TestPicture01.png", pic01)
	put("/TestPicture02.png", pic02)
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
