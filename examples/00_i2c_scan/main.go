// 00_i2c_scan scans the CoreS3 internal I2C bus (SDA=G12, SCL=G11) and prints
// every address that ACKs.
//
// Expected (M0): 0x34 0x36 0x38 0x40 0x51 0x58 0x69 0x23 (0x21 may be absent,
// 0x10 BMM150 is behind the BMI270 and never shows up here).
package main

import (
	"machine"
	"time"

	"esp32-s3-lite/cores3"
)

func main() {
	time.Sleep(2 * time.Second) // give `tinygo monitor` time to attach

	bus := machine.I2C0
	err := bus.Configure(machine.I2CConfig{
		Frequency: 100 * machine.KHz,
		SDA:       cores3.I2C_SDA,
		SCL:       cores3.I2C_SCL,
	})
	if err != nil {
		println("i2c configure error:", err.Error())
	}

	w := []byte{0x00}
	for {
		println("--- I2C scan (SDA=G12 SCL=G11, 100kHz) ---")
		found := 0
		for addr := uint16(0x08); addr < 0x78; addr++ {
			// A read-only transaction does not report NACK on the TinyGo ESP32-S3
			// I2C implementation (the NACK of the last read byte is masked), so probe
			// with a one byte write of register pointer 0x00 instead.
			if bus.Tx(addr, w, nil) == nil {
				print("found 0x")
				printHex(uint8(addr))
				print(" ")
				println(name(addr))
				found++
			}
		}
		println("devices:", found)
		time.Sleep(3 * time.Second)
	}
}

func name(addr uint16) string {
	switch addr {
	case cores3.AddrAXP2101:
		return "AXP2101 (PMIC)"
	case cores3.AddrAW9523B:
		return "AW9523B (IO expander)"
	case cores3.AddrFT6336U:
		return "FT6336U (touch)"
	case cores3.AddrBM8563:
		return "BM8563 (RTC)"
	case cores3.AddrBMI270:
		return "BMI270 (IMU)"
	case cores3.AddrLTR553:
		return "LTR-553ALS (proximity/light)"
	case cores3.AddrGC0308:
		return "GC0308 (camera SCCB)"
	case cores3.AddrES7210:
		return "ES7210 (mic codec)"
	case cores3.AddrAW88298:
		return "AW88298 (speaker amp)"
	}
	return "?"
}

func printHex(v uint8) {
	const hex = "0123456789abcdef"
	print(hex[v>>4:v>>4+1], hex[v&0xf:v&0xf+1])
}
