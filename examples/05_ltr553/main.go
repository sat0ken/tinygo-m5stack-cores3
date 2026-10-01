// 05_ltr553 ports the M5Stack CoreS3 "LTR553" Arduino tutorial: proximity (PS)
// and ambient light (ALS) values, updated every 100 ms.
//
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/ltr553
//
//	LED pulse 40 kHz, PS measurement rate 50 ms, ALS gain 48x, PS and ALS active
//	(the other settings are LTR5XX_BASE_PARA_CONFIG_DEFAULT).
package main

import (
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/drivers/ltr553"
	"esp32-s3-lite/internal/gfx"

	"tinygo.org/x/tinyfont/freemono"
)

var font = &freemono.Bold12pt7b

func main() {
	time.Sleep(2 * time.Second) // give the serial monitor time to attach
	println("=== 05_ltr553 ===")

	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	d, err := cores3.InitDisplay()
	if err != nil {
		println("InitDisplay failed:", err.Error())
	}
	cores3.Verbose = false
	w, h := d.Size()

	sensor := ltr553.New(cores3.I2C)
	cfg := ltr553.DefaultConfig
	cfg.PSLEDPulseFreq = ltr553.LEDPulse40kHz
	cfg.PSMeasurementRate = ltr553.PSMeasRate50ms
	cfg.ALSGain = ltr553.ALSGain48X
	if err := sensor.Configure(cfg); err != nil {
		println("Ltr553 Init Fail:", err.Error())
		gfx.DrawTextCentered(d, font, w/2, h/2, "Ltr553 Init Fail", gfx.Green)
		for {
			time.Sleep(10 * time.Millisecond)
		}
	}
	gfx.DrawTextCentered(d, font, w/2, h/2, "Ltr553 Init Success", gfx.Green)
	part, _ := sensor.ReadRegister(ltr553.RegPartID)
	manu, _ := sensor.ReadRegister(ltr553.RegManufacID)
	println("Ltr553 Init Success, PART_ID:", part, "MANUFAC_ID:", manu)

	sensor.SetPSActive(true)
	sensor.SetALSActive(true)

	var text [40]byte
	// M5.Display.clear() every loop in the tutorial; clearing only the text
	// band gives the same picture without flashing the whole screen.
	lineH := int16(font.YAdvance)
	for {
		ps, _ := sensor.PS()
		als, _ := sensor.ALS()

		s := append(text[:0], "PS:"...)
		s = gfx.Itoa(s, int(ps))
		s = append(s, " / ALS:"...)
		s = gfx.Itoa(s, int(als))
		d.FillRectangle565(0, h/2-lineH, w, 2*lineH, gfx.Black)
		gfx.DrawTextCentered(d, font, w/2, h/2, gfx.String(s), gfx.Green)

		println("ps value =", ps)
		println("als value =", als)
		time.Sleep(100 * time.Millisecond)
	}
}
