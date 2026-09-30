package cores3

import (
	"image/color"
	"machine"
	"time"

	"esp32-s3-lite/drivers/aw9523b"
	"esp32-s3-lite/drivers/ili9342"
)

// LCDFrequency is the SPI write clock of the LCD.
// ref: M5GFX src/M5GFX.cpp (CoreS3: bus_cfg.freq_write = 40000000)
const LCDFrequency = 40 * machine.MHz

// offsetRotation is the rotation offset of the panel on this board: user
// rotation r maps to controller rotation (r + 3) & 3. The default user
// rotation 1 therefore is controller rotation 0 (native 320x240, MADCTL=BGR).
// ref: M5GFX src/M5GFX.cpp (Panel_M5StackCoreS3_T: offset_rotation = 3, _rotation = 1)
const offsetRotation = 3

// DefaultRotation is the M5GFX default rotation for the CoreS3 (landscape,
// USB-C port at the bottom).
const DefaultRotation = 1

var (
	// SPI is the LCD / microSD bus.
	SPI = machine.SPI0

	// Display is the LCD, ready after InitDisplay.
	Display ili9342.Device

	// LCDProbe holds the result of the ILI9342C / E detection.
	LCDProbe ili9342.ProbeResult
)

// InitDisplay detects the LCD controller variant, configures the SPI bus,
// resets the panel through the AW9523B and sends the init sequence. The screen
// is cleared to black. Init must have been called before.
func InitDisplay() (*ili9342.Device, error) {
	// 1. Tell ILI9342C from ILI9342E (the pins are bit-banged for this).
	// ref: M5GFX src/M5GFX.cpp (autodetect: _read_panel_id, _identify_ili9342(&panel_probe, keys, 1))
	LCDProbe = ili9342.ProbeVariant(LCD_SCK, LCD_MOSI, LCD_CS, LCD_DC)
	variant := LCDProbe.Variant
	if Verbose {
		print("cores3: LCD RDDID=0x")
		printHex8(LCDProbe.RDDID)
		print(" (0xe3 expected) DDh=0x")
		printHex8(LCDProbe.Keys[0])
		print(" CBh=0x")
		printHex8(LCDProbe.Keys[1])
		print(" ID4=0x")
		printHex8(LCDProbe.Keys[2])
		printHex8(LCDProbe.Keys[3])
		println(" ->", variant.String())
	}
	if variant == ili9342.VariantUnknown {
		// ref: M5GFX _log_ili9342_variant ("neither key answered, ILI9342C assumed")
		println("cores3: LCD variant not identified, ILI9342C assumed")
		variant = ili9342.VariantC
	}

	// 2. SPI bus. GPIO35 is used only as D/C here (no MISO). microSD access needs
	//    GPIO35 switched to MISO while the LCD CS is high (M5GFX cs_control).
	// ref: M5GFX src/M5GFX.cpp (CoreS3: spi_mode = 0, freq_write = 40 MHz)
	err := SPI.Configure(machine.SPIConfig{
		Frequency: LCDFrequency,
		SCK:       LCD_SCK,
		SDO:       LCD_MOSI,
		SDI:       machine.NoPin,
		Mode:      0,
	})
	if err != nil {
		return nil, err
	}

	// 3. Hardware reset through AW9523B P1_1.
	// ref: M5GFX src/lgfx/v1/panel/Panel_Device.inl (Panel_Device::init: rst_control(false); delay(8); ... rst_control(true); delay(64))
	// ref: M5GFX src/M5GFX.cpp (Panel_M5StackCoreS3_T::rst_control: AW9523B reg 0x03 bit1)
	if err := IOExp.SetPins(aw9523b.Port1, P1_LCD_RST, false); err != nil {
		return nil, err
	}
	time.Sleep(8 * time.Millisecond)
	if err := IOExp.SetPins(aw9523b.Port1, P1_LCD_RST, true); err != nil {
		return nil, err
	}
	time.Sleep(64 * time.Millisecond)

	// 4. Init list, INVON, RGB565, rotation.
	// ref: M5GFX src/M5GFX.cpp (Panel_M5StackCoreS3_T: invert = true, rgb_order default false = BGR)
	Display = ili9342.New(SPI, LCD_DC, LCD_CS)
	err = Display.Configure(ili9342.Config{
		Variant:  variant,
		Rotation: controllerRotation(DefaultRotation),
		Invert:   true,
	})
	if err != nil {
		return nil, err
	}
	Display.FillScreen(color.RGBA{0, 0, 0, 255})
	return &Display, nil
}

// SetRotation sets the display rotation as in M5GFX (0..3, +4 = mirrored).
// 1 is the default landscape orientation.
func SetRotation(r uint8) {
	Display.SetRotation(controllerRotation(r))
}

// controllerRotation adds the board's offset_rotation.
// ref: M5GFX src/lgfx/v1/panel/Panel_LCD.inl (Panel_LCD::setRotation:
//
//	_internal_rotation = ((r + offset_rotation) & 3) | ((r & 4) ^ (offset_rotation & 4)))
func controllerRotation(r uint8) uint8 {
	return ((r + offsetRotation) & 3) | ((r & 4) ^ (offsetRotation & 4))
}
