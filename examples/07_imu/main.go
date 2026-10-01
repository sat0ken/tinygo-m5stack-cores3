// 07_imu ports the M5Stack CoreS3 "IMU" Arduino tutorial: accelerometer,
// gyroscope and magnetometer (9 values) every 100 ms.
//
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/imu
//
// Units follow M5Unified: accel in G, gyro in deg/s, mag raw * 10*4912/32760.
package main

import (
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/internal/gfx"

	"tinygo.org/x/tinyfont/freemono"
)

func main() {
	time.Sleep(2 * time.Second) // give the serial monitor time to attach
	println("=== 07_imu ===")

	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	d, err := cores3.InitDisplay()
	if err != nil {
		println("InitDisplay failed:", err.Error())
	}
	if err := cores3.InitIMU(); err != nil {
		println("InitIMU failed:", err.Error())
	}
	cores3.Verbose = false
	println("magnetometer:", cores3.IMU.HasMag)

	con := gfx.Console{D: d, Font: &freemono.Bold9pt7b, Fore: gfx.White, Back: gfx.Black}
	w, h := d.Size()
	var line [64]byte
	for {
		if cores3.IMU.Update() {
			con.SetCursor(0, 20)
			d.FillRectangle565(0, 0, w, h, gfx.Black) // M5.Display.clear()

			v := &cores3.IMU.Data
			serial3(line[:0], "ax:", "  ay:", "  az:", v.Accel)
			serial3(line[:0], "gx:", "  gy:", "  gz:", v.Gyro)
			serial3(line[:0], "mx:", "  my:", "  mz:", v.Mag)

			con.Print("IMU:\r\n")
			screen3(&con, line[:0], "ax:", "ay:", "az:", v.Accel)
			screen3(&con, line[:0], "gx:", "gy:", "gz:", v.Gyro)
			screen3(&con, line[:0], "mx:", "my:", "mz:", v.Mag)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// serial3 prints "ax:%f  ay:%f  az:%f".
func serial3(b []byte, l0, l1, l2 string, v [3]float32) {
	b = append(b, l0...)
	b = gfx.AppendFloat(b, v[0])
	b = append(b, l1...)
	b = gfx.AppendFloat(b, v[1])
	b = append(b, l2...)
	b = gfx.AppendFloat(b, v[2])
	println(gfx.String(b))
}

// screen3 prints "ax:%f\nay:%f\naz:%f\r\n".
//
// Note: a [3]string indexed with a loop variable here made append() request
// a huge allocation ("out of memory") on TinyGo 0.42 / Xtensa; see
// docs/notes.md. The labels are passed one by one instead.
func screen3(con *gfx.Console, b []byte, l0, l1, l2 string, v [3]float32) {
	screenLine(con, b, l0, v[0])
	screenLine(con, b, l1, v[1])
	screenLine(con, b, l2, v[2])
}

func screenLine(con *gfx.Console, b []byte, label string, v float32) {
	b = append(b[:0], label...)
	b = gfx.AppendFloat(b, v)
	b = append(b, '\n')
	con.Print(gfx.String(b))
}
