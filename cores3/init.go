package cores3

import (
	"device/esp"
	"machine"
	"time"

	"esp32-s3-lite/drivers/aw9523b"
	"esp32-s3-lite/drivers/axp2101"
)

// I2CFrequency is the internal I2C bus clock.
// ref: M5GFX src/M5GFX.cpp (ESP32S3: i2c_freq = 400000)
const I2CFrequency = 400 * machine.KHz

var (
	// I2C is the internal I2C bus (G12/G11), configured by Init. Use it (not
	// machine.I2C0 directly) for the on-board chips: see i2c.go.
	I2C = &i2cBus{bus: machine.I2C0, reg: esp.I2C0}

	// PMIC is the AXP2101, ready after Init.
	PMIC axp2101.Device
	// IOExp is the AW9523B, ready after Init.
	IOExp aw9523b.Device
)

// Verbose makes Init print every register it writes (and its read-back).
var Verbose = true

// Init configures the internal I2C bus and powers up the board in the same
// order and with the same values as M5GFX (display autodetect) followed by
// M5Unified (Power.begin) do for the CoreS3.
//
// After Init the LCD backlight is on (DLDO1), and all peripheral rails
// (ALDO1..4) are enabled. The LCD itself is not initialized: see InitDisplay.
func Init() error {
	// Deselect the SPI devices first: a floating SD CS lets the microSD card
	// take bus traffic for commands (see docs/notes.md).
	for _, cs := range [...]machine.Pin{SD_CS, LCD_CS} {
		cs.Configure(machine.PinConfig{Mode: machine.PinOutput})
		cs.High()
	}

	I2C.cfg = machine.I2CConfig{
		Frequency: I2CFrequency,
		SDA:       I2C_SDA,
		SCL:       I2C_SCL,
	}
	if err := I2C.configure(); err != nil {
		return err
	}

	IOExp = aw9523b.New(I2C)
	PMIC = axp2101.New(I2C)
	if err := IOExp.Configure(); err != nil {
		println("cores3: AW9523B not found:", err.Error())
		return err
	}
	if err := PMIC.Configure(); err != nil {
		println("cores3: AXP2101 not found:", err.Error())
		return err
	}

	// ---- Step 1: M5GFX autodetect (board_M5StackCoreS3) ----
	// ref: M5GFX src/M5GFX.cpp M5GFX::autodetect(), ESP32S3 / pkg_ver 0 / CoreS3 branch

	// If the SPI bus pull-ups are not effective (all three lines read low),
	// M5GFX turns on the 5V BUS output as well (some modules sink the lines
	// while 5V is off).
	pullupOK := spiPullupsHigh()
	var p0, p1 uint8 = 0b00000101, 0b00000011
	if !pullupOK {
		p0, p1 = 0b00000111, 0b10000011
	}
	if Verbose {
		println("cores3: SPI line pull-up check (G35/G36/G37):", pullupOK)
	}

	steps := [...]struct {
		dev  uint8 // 0 = AW9523B, 1 = AXP2101
		op   uint8 // 0 = write, 1 = set bits, 2 = clear bits
		reg  uint8
		val  uint8
		note string
	}{
		{0, 1, aw9523b.RegOutputP0, p0, "P0 out: TOUCH_RST, AW88298_RST high (+BUS_OUT_EN)"},
		{0, 1, aw9523b.RegOutputP1, p1, "P1 out: CAM_RST, LCD_RST high (+BOOST_EN)"},
		{0, 0, aw9523b.RegConfigP0, 0b00011000, "CONFIG_P0 (P0_3, P0_4 input)"},
		{0, 0, aw9523b.RegConfigP1, 0b00001100, "CONFIG_P1 (P1_2 TOUCH_INT, P1_3 AW88298_INT input)"},
		{0, 0, aw9523b.RegGCR, 0b00010000, "GCR: P0 push-pull"},
		{0, 0, aw9523b.RegLEDModeP0, 0b11111111, "LEDMODE_P0: all GPIO"},
		{0, 0, aw9523b.RegLEDModeP1, 0b11111111, "LEDMODE_P1: all GPIO"},
		// A DCDC dip below 85% makes the AXP2101 power itself off; let the SoC
		// brown out and restart instead.
		{1, 2, axp2101.RegDCDCUVP, 0x1F, "DCDC1-5 UVP power-off disable"},
		{1, 0, axp2101.RegLDOEnable0, 0xBF, "LDO enable: ALDO1-4, BLDO1-2, DLDO1"},
		{1, 0, axp2101.RegALDO1 + 2, 33 - 5, "ALDO3 3.3V (camera)"},
		{1, 0, axp2101.RegALDO1 + 3, 33 - 5, "ALDO4 3.3V (TF card)"},

		// ---- Step 2: M5Unified Power_Class::begin (board_M5StackCoreS3) ----
		// ref: M5Unified src/utility/Power_Class.inl (CONFIG_IDF_TARGET_ESP32S3 / board_M5StackCoreS3)
		{0, 1, aw9523b.RegOutputP1, P1_BOOST_EN, "SY7088 BOOST_EN"},
		{1, 2, axp2101.RegDCDCUVP, 0x1F, "DCDC1-5 UVP power-off disable"},
		{1, 0, axp2101.RegLDOEnable0, 0xBF, "LDO enable: ALDO1-4, BLDO1-2, DLDO1"},
		{1, 0, axp2101.RegALDO1 + 0, 18 - 5, "ALDO1 1.8V (AW88298)"},
		{1, 0, axp2101.RegALDO1 + 1, 33 - 5, "ALDO2 3.3V (ES7210)"},
		{1, 0, axp2101.RegALDO1 + 2, 33 - 5, "ALDO3 3.3V (camera)"},
		{1, 0, axp2101.RegALDO1 + 3, 33 - 5, "ALDO4 3.3V (TF card)"},
		{1, 0, axp2101.RegPwronKey, 0x00, "PowerKey hold=1s / poweroff=4s"},
		{1, 0, axp2101.RegCHGLED, 0x11, "CHGLED setting"},
		{1, 0, axp2101.RegCommonConfig, 0x30, "PMU common config"},
		{1, 0, axp2101.RegADCEnable, 0x0F, "ADC enable"},
	}

	for _, s := range steps {
		var err error
		var rb uint8
		if s.dev == 0 {
			err = regOp(IOExp.WriteRegister, IOExp.SetBits, IOExp.ClearBits, s.op, s.reg, s.val)
			if err == nil {
				rb, err = IOExp.ReadRegister(s.reg)
			}
		} else {
			err = regOp(PMIC.WriteRegister, PMIC.SetBits, PMIC.ClearBits, s.op, s.reg, s.val)
			if err == nil {
				rb, err = PMIC.ReadRegister(s.reg)
			}
		}
		if Verbose {
			if s.dev == 0 {
				print("  AW9523B ")
			} else {
				print("  AXP2101 ")
			}
			print(opName[s.op], " 0x")
			printHex8(s.reg)
			print(" 0x")
			printHex8(s.val)
			print(" -> read 0x")
			printHex8(rb)
			println("  //", s.note)
		}
		if err != nil {
			println("cores3: i2c error:", err.Error())
			return err
		}
	}

	// Backlight on at the M5GFX default brightness.
	// ref: M5GFX src/lgfx/v1/LGFXBase.hpp (_brightness = 127)
	return SetBrightness(127)
}

var opName = [...]string{"write ", "setbit", "clrbit"}

func regOp(w, set, clr func(reg, v uint8) error, op, reg, v uint8) error {
	switch op {
	case 1:
		return set(reg, v)
	case 2:
		return clr(reg, v)
	}
	return w(reg, v)
}

// SetBrightness sets the LCD backlight (DLDO1). 0 turns it off.
// ref: M5GFX src/M5GFX.cpp Light_M5StackCoreS3::setBrightness
//
//	brightness = ((brightness + 641) >> 5); bitOn(0x90, 0x80); writeRegister8(0x99, brightness)
func SetBrightness(brightness uint8) error {
	if brightness == 0 {
		return PMIC.ClearBits(axp2101.RegLDOEnable0, 0x80)
	}
	v := uint8((uint16(brightness) + 641) >> 5)
	if err := PMIC.SetBits(axp2101.RegLDOEnable0, 0x80); err != nil {
		return err
	}
	err := PMIC.WriteRegister(axp2101.RegDLDO1, v)
	if Verbose {
		rb, _ := PMIC.ReadRegister(axp2101.RegDLDO1)
		print("  AXP2101 write  0x99 0x")
		printHex8(v)
		print(" -> read 0x")
		printHex8(rb)
		println("  // DLDO1 backlight, brightness", brightness)
	}
	return err
}

// spiPullupsHigh reads G35/G36/G37 with the internal pull-ups on.
// ref: M5GFX src/M5GFX.cpp (lgfx::gpio::command: input_pullup 35,36,37 then read; result == 0 -> enable VBUS 5V)
func spiPullupsHigh() bool {
	pins := [...]machine.Pin{LCD_DC, LCD_SCK, LCD_MOSI}
	for _, p := range pins {
		p.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	}
	time.Sleep(time.Millisecond)
	high := false
	for _, p := range pins {
		if p.Get() {
			high = true
		}
	}
	return high
}

// DumpRegisters prints the power related registers of the AXP2101 and AW9523B.
func DumpRegisters() {
	println("--- AW9523B ---")
	for _, r := range [...]uint8{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x10, 0x11, 0x12, 0x13} {
		v, err := IOExp.ReadRegister(r)
		dumpLine(r, v, err)
	}
	println("--- AXP2101 ---")
	for _, r := range [...]uint8{0x00, 0x01, 0x03, 0x10, 0x18, 0x23, 0x27, 0x30, 0x62, 0x69, 0x90, 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x99, 0xA4} {
		v, err := PMIC.ReadRegister(r)
		dumpLine(r, v, err)
	}
}

func dumpLine(r, v uint8, err error) {
	print("  reg 0x")
	printHex8(r)
	print(" = 0x")
	printHex8(v)
	if err != nil {
		print("  err: ", err.Error())
	}
	println()
}

func printHex8(v uint8) {
	const hex = "0123456789abcdef"
	print(hex[v>>4:v>>4+1], hex[v&0xf:v&0xf+1])
}
