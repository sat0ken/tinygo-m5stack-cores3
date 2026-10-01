// 04_rtc ports the M5Stack CoreS3 "RTC" Arduino tutorial without NTP: the RTC
// (BM8563) is set directly, as in the commented-out "direct setting" block of
// the tutorial, and the RTC time, the ESP32 clock (UTC) and the ESP32 clock in
// local time are printed every 500 ms.
//
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/rtc
//
// The RTC keeps UTC. It is set when:
//   - setTime is given at build time (RFC 3339, UTC), e.g.
//     tinygo flash -target=esp32s3-generic -ldflags="-X main.setTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" ./examples/04_rtc
//   - or the RTC reports VL (time not valid): it is set to 2021-12-31 12:34:56,
//     the value in the tutorial.
//
// The tutorial shows the local time of "UTC-8" (POSIX TZ, i.e. UTC+8). This
// port shows JST (UTC+9) instead.
package main

import (
	"time"

	"esp32-s3-lite/cores3"
	"esp32-s3-lite/internal/gfx"

	"tinygo.org/x/tinyfont/freemono"
)

var setTime string // set with -ldflags "-X main.setTime=..."

var wd = [7]string{"Sun", "Mon", "Tue", "Wed", "Thr", "Fri", "Sat"} // as in the tutorial

var jst = time.FixedZone("JST", 9*60*60)

func main() {
	time.Sleep(2 * time.Second) // give the serial monitor time to attach
	println("=== 04_rtc ===")

	if err := cores3.Init(); err != nil {
		println("Init failed:", err.Error())
	}
	d, err := cores3.InitDisplay()
	if err != nil {
		println("InitDisplay failed:", err.Error())
	}
	cores3.Verbose = false
	con := gfx.Console{D: d, Font: &freemono.Bold12pt7b, Fore: gfx.White, Back: gfx.Black}

	if err := cores3.InitRTC(); err != nil {
		println("RTC not found.")
		for {
			time.Sleep(500 * time.Millisecond)
		}
	}
	println("RTC found.")

	vl, _ := cores3.RTC.VoltLow()
	println("RTC VL (time invalid) flag:", vl)
	switch {
	case setTime != "":
		t, err := time.Parse(time.RFC3339, setTime)
		if err != nil {
			println("bad setTime:", setTime)
			break
		}
		cores3.RTC.SetTime(t.UTC())
		println("RTC set from build time:", setTime)
	case vl:
		//                         YYYY  MM  DD      hh  mm  ss
		cores3.RTC.SetTime(time.Date(2021, 12, 31, 12, 34, 56, 0, time.UTC))
		println("RTC set to 2021-12-31 12:34:56 (tutorial value)")
	}

	if _, err := cores3.SyncSystemTime(); err != nil {
		println("RTC read failed:", err.Error())
	}

	var line [64]byte
	for {
		time.Sleep(500 * time.Millisecond)

		rt, err := cores3.RTC.ReadTime()
		if err != nil {
			println("RTC read failed:", err.Error())
			continue
		}
		con.SetCursor(0, 0)

		s := format(line[:0], "RTC   UTC  :", rt)
		println(gfx.String(s))
		con.Print(gfx.String(s))
		con.Print("\n")

		now := time.Now()
		s = format(line[:0], "ESP32 UTC  :", now.UTC())
		println(gfx.String(s))
		con.Print(gfx.String(s))
		con.Print("\n")

		s = format(line[:0], "ESP32 JST-9:", now.In(jst))
		println(gfx.String(s))
		con.Print(gfx.String(s))
		con.Print("\n")
	}
}

// format appends "label YYYY/MM/DD (Www)  hh:mm:ss" like the tutorial's printf.
// The weekday is computed from the date.
func format(b []byte, label string, t time.Time) []byte {
	b = append(b, label...)
	b = gfx.Append4(b, t.Year())
	b = append(b, '/')
	b = gfx.Append2(b, int(t.Month()))
	b = append(b, '/')
	b = gfx.Append2(b, t.Day())
	b = append(b, " ("...)
	b = append(b, wd[t.Weekday()]...)
	b = append(b, ")  "...)
	b = gfx.Append2(b, t.Hour())
	b = append(b, ':')
	b = gfx.Append2(b, t.Minute())
	b = append(b, ':')
	b = gfx.Append2(b, t.Second())
	return b
}
