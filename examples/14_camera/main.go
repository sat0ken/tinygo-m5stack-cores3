// 14_camera shows a live preview from the GC0308 camera. Touching the screen
// starts a 3-2-1 countdown (shown over the preview, so the finger is out of
// the picture when the shot is taken); then the frame is frozen and saved on
// the microSD card as /CAM_nnnn.BMP (24 bit BMP, the first unused number;
// existing files are never overwritten).
//
// The M5Stack tutorial streams video with two frame buffers in PSRAM, which
// TinyGo does not support; here one frame buffer in internal RAM is captured
// and drawn in turn (about 6-10 fps). See docs/notes.md.
//
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/camera
package main

import (
	"bufio"
	"os"
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/internal/bmp"
	"esp32-s3-lite/internal/gfx"

	"tinygo.org/x/tinyfont/freesans"
)

var (
	photo  [cores3.PhotoWidth * cores3.PhotoHeight * 2]byte
	bmpRow [cores3.PhotoWidth * 3]byte
	name   [32]byte
	font   = &freesans.Regular12pt7b
	sdOK   bool
	wbuf   *bufio.Writer
)

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

	if err := cores3.InitSD(); err != nil {
		println("InitSD failed (photos will not be saved):", err.Error())
	} else {
		sdOK = true
	}

	if err := cores3.InitCamera(); err != nil {
		println("InitCamera failed:", err.Error())
		gfx.DrawTextCentered(d, font, w/2, h/2, "Camera Init Fail", gfx.Red)
		for {
			time.Sleep(time.Second)
		}
	}
	println("Camera Init Success")
	gfx.DrawTextCentered(d, font, w/2, h/2, "Touch to save a photo", gfx.White)

	time.Sleep(time.Second)
	start := time.Now()
	touched := func() bool {
		cores3.Touch.Update(uint32(time.Since(start).Milliseconds()))
		return cores3.Touch.Detail().WasPressed() // touch_begin: react to the start of a touch
	}
	frames := 0
	fpsStart := time.Now()
	var countdownStart time.Time
	counting := false
	for {
		if !counting && touched() {
			counting, countdownStart = true, time.Now()
			println("countdown")
		}
		if err := cores3.CapturePhoto(photo[:]); err != nil {
			println("capture failed:", err.Error())
			continue
		}
		save := false
		if counting {
			left := 3 - int(time.Since(countdownStart)/time.Second) // 3, 2, 1, then 0 = shoot
			if left <= 0 {
				// this frame was captured after the countdown: keep it as the photo
				counting, save = false, true
			} else {
				d.DrawRGBBitmap8(0, 0, photo[:], cores3.PhotoWidth, cores3.PhotoHeight)
				drawCount(d, w, h, left)
			}
		}
		if !save {
			if !counting {
				d.DrawRGBBitmap8(0, 0, photo[:], cores3.PhotoWidth, cores3.PhotoHeight)
			}
			frames++
			if frames%50 == 0 {
				ms := time.Since(fpsStart).Milliseconds()
				println("preview:", 50*1000/ms, "fps")
				fpsStart = time.Now()
			}
			if !counting && touched() {
				counting, countdownStart = true, time.Now()
				println("countdown")
			}
			continue
		}
		d.DrawRGBBitmap8(0, 0, photo[:], cores3.PhotoWidth, cores3.PhotoHeight)
		if !sdOK {
			gfx.DrawTextCentered(d, font, w/2, h-16, "No SD card", gfx.Red)
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		gfx.DrawTextCentered(d, font, w/2, h-16, "Saving...", gfx.Yellow)
		t2 := time.Now()
		path, err := savePhoto()
		// redraw the bottom lines that the message covered
		d.DrawRGBBitmap8(0, h-32, photo[(int(h)-32)*int(w)*2:], w, 32)
		if err != nil {
			println("save failed:", err.Error())
			gfx.DrawTextCentered(d, font, w/2, h-16, "Save failed", gfx.Red)
		} else {
			println("saved", path, "in", time.Since(t2).Milliseconds(), "ms")
			gfx.DrawTextCentered(d, font, w/2, h-16, path[1:], gfx.Green)
		}
		time.Sleep(1500 * time.Millisecond)
		fpsStart = time.Now()
		frames = 0
	}
}

var countFont = &freesans.Bold24pt7b

// drawCount draws the countdown digit in the middle of the screen, yellow
// with a black outline so that it reads on any picture.
func drawCount(d gfx.Filler, w, h int16, n int) {
	digit := "0123456789"[n : n+1]
	for _, o := range [...][2]int16{{-2, 0}, {2, 0}, {0, -2}, {0, 2}, {-2, -2}, {2, 2}, {-2, 2}, {2, -2}} {
		gfx.DrawTextCentered(d, countFont, w/2+o[0], h/2+o[1], digit, gfx.Black)
	}
	gfx.DrawTextCentered(d, countFont, w/2, h/2, digit, gfx.Yellow)
}

// savePhoto writes the photo to the first unused /CAM_nnnn.BMP.
func savePhoto() (string, error) {
	var path string
	for n := 1; n <= 9999; n++ {
		b := append(name[:0], "/CAM_"...)
		b = append(b, byte('0'+n/1000%10), byte('0'+n/100%10), byte('0'+n/10%10), byte('0'+n%10))
		b = append(b, ".BMP"...)
		if _, err := cores3.FS.Stat(gfx.String(b)); err != nil {
			path = string(b) // copy: name is reused
			break
		}
	}
	if path == "" {
		return "", os.ErrExist
	}
	f, err := cores3.FS.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return "", err
	}
	if wbuf == nil {
		wbuf = bufio.NewWriterSize(f, 4096)
	} else {
		wbuf.Reset(f)
	}
	err = bmp.WriteRGB565BE(wbuf, photo[:], cores3.PhotoWidth, cores3.PhotoHeight, bmpRow[:])
	if err == nil {
		err = wbuf.Flush()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	if fi, err := cores3.FS.Stat(path); err != nil || fi.Size() != int64(bmp.Size(cores3.PhotoWidth, cores3.PhotoHeight)) {
		return "", os.ErrInvalid
	}
	return path, nil
}
