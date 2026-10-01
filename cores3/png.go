package cores3

import (
	"bufio"

	"esp32-s3-lite/internal/pngstream"
)

var (
	pngDec = pngstream.Decoder{MaxWidth: 320}
	pngBuf *bufio.Reader
)

// DrawPNGFile draws a PNG file from the microSD card with its top-left corner
// at (x, y), like M5.Display.drawPngFile(SD, path). Rows outside the screen
// are skipped. InitSD must have been called.
func DrawPNGFile(path string, x, y int16) error {
	f, err := FS.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if pngBuf == nil {
		pngBuf = bufio.NewReaderSize(f, 4096)
	} else {
		pngBuf.Reset(f)
	}
	w, h := Display.Size()
	_, err = pngDec.Decode(pngBuf, func(row int, rgb565 []byte) error {
		py := y + int16(row)
		if py < 0 || py >= h || x >= w {
			return nil
		}
		n := int16(len(rgb565) / 2)
		if x+n > w {
			n = w - x
		}
		return Display.DrawRGBBitmap8(x, py, rgb565[:n*2], n, 1)
	})
	return err
}
