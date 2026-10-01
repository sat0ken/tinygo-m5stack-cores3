// 06_power ports the M5Stack CoreS3 "Power" Arduino tutorial: sets the charge
// current to 200 mA and shows the charging state, battery voltage, battery
// level and VBUS voltage every second.
//
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/power
//
// Differences:
//   - The tutorial runs M5.begin() with cfg.output_power = true (default),
//     which also turns on the 5 V output to Grove / M-BUS (BUS_OUT_EN). This
//     port leaves the 5 V output as Init set it (off), see docs/notes.md.
//   - The default 6x8 font with setTextSize(2) is replaced by FreeMonoBold 9pt.
package main

import (
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/internal/gfx"

	"tinygo.org/x/tinyfont/freemono"
)

func main() {
	time.Sleep(2 * time.Second) // give the serial monitor time to attach
	println("=== 06_power ===")

	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	d, err := cores3.InitDisplay()
	if err != nil {
		println("InitDisplay failed:", err.Error())
	}
	cores3.Verbose = false

	pmic := &cores3.PMIC
	// ref: M5Unified Power_Class::setChargeCurrent -> AXP2101_Class::setChargeCurrent
	if err := pmic.SetChargeCurrent(200); err != nil {
		println("setChargeCurrent failed:", err.Error())
	}
	v, _ := pmic.ReadRegister(0x62)
	println("charge current reg 0x62 =", v, "(8 = 200 mA)")

	con := gfx.Console{D: d, Font: &freemono.Bold9pt7b, Fore: gfx.White, Back: gfx.Black}
	w, h := d.Size()
	var line [40]byte
	for {
		d.FillRectangle565(0, 0, w, h, gfx.Black)

		charging, _ := pmic.IsCharging()
		mv, _ := pmic.BatteryVoltage()
		level, _ := pmic.BatteryLevel()
		vbus, _ := pmic.VBUSVoltage()

		c := 0
		if charging {
			c = 1
		}
		print1(&con, line[:0], 10, 30, "Bat Charging: ", c, "")
		print1(&con, line[:0], 10, 50, "Bat Voltage: ", int(mv), "mv")
		print1(&con, line[:0], 10, 70, "Bat Level: ", int(level), "")
		print1(&con, line[:0], 10, 90, "VBus Voltage: ", int(vbus), "mv")

		time.Sleep(time.Second)
	}
}

func print1(con *gfx.Console, b []byte, x, y int16, label string, v int, unit string) {
	b = append(b, label...)
	b = gfx.Itoa(b, v)
	b = append(b, unit...)
	con.SetCursor(x, y)
	con.Print(gfx.String(b))
	println(gfx.String(b))
}
