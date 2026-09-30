// Package cores3 is a thin board support package for the M5Stack CoreS3 /
// CoreS3 Lite (ESP32-S3). Chip drivers live in ../drivers; everything that
// depends on how the chips are wired on this board lives here.
//
// ref: https://docs.m5stack.com/ja/core/CoreS3 (pin map)
// ref: https://m5stack-doc.oss-cn-shenzhen.aliyuncs.com/490/Sch_M5_CoreS3_v1.0.pdf
package cores3

import "machine"

// Internal I2C bus (all on-board sensors / PMIC / IO expander).
// ref: M5GFX src/M5GFX.cpp (ESP32S3 section: i2c_sda = GPIO_NUM_12, i2c_scl = GPIO_NUM_11)
const (
	I2C_SDA = machine.GPIO12
	I2C_SCL = machine.GPIO11

	// I2C_INT is the open-drain INTN output of the AW9523B. Touch INT (AW9523B P1_2)
	// reaches the ESP32-S3 through it.
	// ref: M5Unified src/utility/Power_Class.inl (_wakeupPin = GPIO_NUM_21 // I2C_INT ( AW9523 INTN ))
	// ref: M5GFX src/M5GFX.cpp (Touch_M5StackCoreS3: pin_int = GPIO_NUM_21)
	I2C_INT = machine.GPIO21
)

// I2C addresses on the internal bus.
// ref: https://docs.m5stack.com/ja/core/CoreS3 (I2C address table)
const (
	AddrAXP2101 = 0x34
	AddrAW9523B = 0x58
	AddrFT6336U = 0x38
	AddrBM8563  = 0x51
	AddrBMI270  = 0x69
	AddrBMM150  = 0x10 // behind the BMI270 auxiliary I2C, not on the internal bus
	AddrLTR553  = 0x23
	AddrGC0308  = 0x21
	AddrES7210  = 0x40
	AddrAW88298 = 0x36
)

// LCD (ILI9342C / ILI9342E) on SPI.
// ref: M5GFX src/M5GFX.cpp (CoreS3: pin_mosi = 37, pin_miso = 35, pin_sclk = 36, pin_dc = 35, pin_cs = 3)
const (
	LCD_SCK  = machine.GPIO36
	LCD_MOSI = machine.GPIO37
	LCD_CS   = machine.GPIO3
	// LCD_DC is shared with SD_MISO. M5GFX switches GPIO35 between "GPIO out (D/C)"
	// and "FSPIQ (MISO)" in cs_control(). See display.go.
	LCD_DC = machine.GPIO35
)

// microSD shares the LCD SPI bus.
// ref: M5Unified src/M5Unified.inl (board_M5StackCoreS3: GPIO_NUM_36, GPIO_NUM_37, GPIO_NUM_35, ..., GPIO_NUM_4)
const (
	SD_SCK  = machine.GPIO36
	SD_MOSI = machine.GPIO37
	SD_MISO = machine.GPIO35
	SD_CS   = machine.GPIO4
)

// I2S audio (speaker AW88298 / mic ES7210).
// ref: M5Unified src/M5Unified.inl (CoreS3 mic_cfg: mck=0, bck=34, ws=33, data_in=14)
const (
	I2S_MCLK     = machine.GPIO0
	I2S_BCK      = machine.GPIO34
	I2S_WS       = machine.GPIO33
	I2S_DATA_OUT = machine.GPIO13 // speaker (AW88298)
	I2S_DATA_IN  = machine.GPIO14 // microphone (ES7210)
)

// Grove ports.
// ref: https://docs.m5stack.com/ja/core/CoreS3 (PORT.A/B/C)
const (
	PORTA_SDA = machine.GPIO2
	PORTA_SCL = machine.GPIO1
	PORTB_IN  = machine.GPIO9
	PORTB_OUT = machine.GPIO8
	PORTC_RX  = machine.GPIO18
	PORTC_TX  = machine.GPIO17
)

// AW9523B port bits used on CoreS3.
// ref: https://docs.m5stack.com/ja/core/CoreS3 (AW9523B pin assignment)
// ref: M5Unified src/utility/Power_Class.inl (_core_s3_bus_en = 0b00000010, _core_s3_usb_en = 0b00100000,
//
//	_core_s3_aw9523_bit(0x03, 0b10000000, true) // SY7088 BOOST_EN)
//
// ref: M5GFX src/M5GFX.cpp (lcd_rst_bit = 1 << 1; // AW9523B P1_1)
const (
	// Port 0
	P0_TOUCH_RST   = 1 << 0
	P0_BUS_OUT_EN  = 1 << 1
	P0_AW88298_RST = 1 << 2
	P0_USB_OTG_EN  = 1 << 5

	// Port 1
	P1_CAM_RST     = 1 << 0
	P1_LCD_RST     = 1 << 1
	P1_TOUCH_INT   = 1 << 2
	P1_AW88298_INT = 1 << 3
	P1_BOOST_EN    = 1 << 7
)
