// Package ili9342 implements a driver for the ILI9342C and ILI9342E 320x240
// TFT LCD controllers over a 4-wire SPI bus (SCK, SDA, CS, D/C).
//
// The two controllers share the geometry and the standard (MIPI DCS) command
// set, but their power / gamma registers differ and the ILI9342C init list
// must not be sent to an ILI9342E. The init lists are taken from M5GFX, which
// supports both on the M5Stack Core2 / CoreS3. See ProbeVariant to tell them
// apart on the bus.
//
// ref: M5GFX src/lgfx/v1/panel/Panel_ILI9342.hpp
// ref: M5GFX src/lgfx/v1/panel/Panel_LCD.inl / Panel_LCD.hpp
package ili9342

import (
	"errors"
	"image/color"
	"machine"
	"time"

	"tinygo.org/x/drivers"
)

// Variant selects the controller.
type Variant uint8

const (
	VariantUnknown Variant = iota
	VariantC               // ILI9342C
	VariantE               // ILI9342E
)

func (v Variant) String() string {
	switch v {
	case VariantC:
		return "ILI9342C"
	case VariantE:
		return "ILI9342E"
	}
	return "unknown"
}

// Standard commands.
// ref: M5GFX src/lgfx/v1/panel/Panel_LCD.hpp
const (
	NOP     = 0x00
	SWRESET = 0x01
	RDDID   = 0x04
	SLPIN   = 0x10
	SLPOUT  = 0x11
	INVOFF  = 0x20
	INVON   = 0x21
	DISPOFF = 0x28
	DISPON  = 0x29
	CASET   = 0x2A
	PASET   = 0x2B
	RAMWR   = 0x2C
	MADCTL  = 0x36
	IDMOFF  = 0x38
	COLMOD  = 0x3A
)

// MADCTL bits.
const (
	MAD_MY  = 0x80
	MAD_MX  = 0x40
	MAD_MV  = 0x20
	MAD_ML  = 0x10
	MAD_BGR = 0x08
	MAD_MH  = 0x04
)

// madctlTable maps a rotation (0..7, 4..7 = mirrored) to MADCTL.
// ref: M5GFX src/lgfx/v1/panel/Panel_LCD.hpp (Panel_LCD::getMadCtl, madctl_table)
var madctlTable = [8]uint8{
	0,
	MAD_MV | MAD_MX | MAD_MH,
	MAD_MX | MAD_MH | MAD_MY | MAD_ML,
	MAD_MV | MAD_MY | MAD_ML,
	MAD_MY | MAD_ML,
	MAD_MV,
	MAD_MX | MAD_MH,
	MAD_MV | MAD_MX | MAD_MY | MAD_MH | MAD_ML,
}

// Init command lists: cmd, n, args..., with n|0x80 meaning "then wait 120 ms",
// terminated by 0xFF 0xFF.
// ref: M5GFX src/lgfx/v1/panel/Panel_ILI9342.hpp (Panel_ILI9342::getInitCommands)
var initC = []byte{
	0xC8, 3, 0xFF, 0x93, 0x42, // SETEXTC: turn on the external command
	0xC0, 2, 0x12, 0x12, // PWCTR1
	0xC1, 1, 0x03, // PWCTR2
	0xC5, 1, 0xF2, // VMCTR1
	0xB0, 1, 0xE0,
	0xF6, 3, 0x01, 0x00, 0x00,
	0xE0, 15, 0x00, 0x0C, 0x11, 0x04, 0x11, 0x08, 0x37, 0x89, 0x4C, 0x06, 0x0C, 0x0A, 0x2E, 0x34, 0x0F, // GMCTRP1
	0xE1, 15, 0x00, 0x0B, 0x11, 0x05, 0x13, 0x09, 0x33, 0x67, 0x48, 0x07, 0x0E, 0x0B, 0x2E, 0x33, 0x0F, // GMCTRN1
	0xB6, 4, 0x08, 0x82, 0x1D, 0x04, // DFUNCTR
	IDMOFF, 0,
	DISPON, 0,
	SLPOUT, 0x80, // exit sleep, wait 120 ms
	0xFF, 0xFF,
}

// ref: M5GFX src/lgfx/v1/panel/Panel_ILI9342.hpp (Panel_ILI9342E::getInitCommands)
var initE = []byte{
	0xDD, 1, 0x01, // SETEXTC (E): turn on the external command
	0xD5, 1, 0x00,
	0xB1, 1, 0x22,
	0xC8, 1, 0x38, // Power Control 3 (VCOM)
	0xCB, 1, 0x1C,
	0xC9, 1, 0x1A,
	0xCA, 1, 0x1A,
	0xB7, 4, 0x5A, 0x41, 0x11, 0x19,
	0xE4, 15, 0x04, 0x08, 0x11, 0x06, 0x12, 0x07, 0x3A, 0x76, 0x47, 0x07, 0x0F, 0x0A, 0x11, 0x19, 0x05, // GMCTRP1 (E)
	0xE5, 15, 0x02, 0x03, 0x07, 0x06, 0x12, 0x07, 0x36, 0x5F, 0x48, 0x06, 0x10, 0x0C, 0x16, 0x14, 0x09, // GMCTRN1 (E)
	IDMOFF, 0,
	DISPON, 0,
	SLPOUT, 0x80, // exit sleep, wait 120 ms
	0xFF, 0xFF,
}

// Config is the display configuration.
type Config struct {
	// Variant must be VariantC or VariantE.
	Variant Variant
	// Rotation is the controller rotation 0..7 as in M5GFX (after adding the
	// board's offset_rotation). 0 = native 320x240 landscape.
	Rotation uint8
	// Invert sends INVON instead of INVOFF.
	Invert bool
	// RGB selects RGB order; the default (false) is BGR.
	RGB bool
}

// Device is an ILI9342 on a SPI bus.
type Device struct {
	bus      drivers.SPI
	dc, cs   machine.Pin
	width    int16
	height   int16
	rotation uint8
	variant  Variant
	rgb      bool
	cmdBuf   [4]byte
	fillBuf  [256]byte
}

var ErrVariant = errors.New("ili9342: variant must be C or E")

// New returns a Device. The bus must already be configured (mode 0). cs may
// be machine.NoPin when the chip select is handled elsewhere.
func New(bus drivers.SPI, dc, cs machine.Pin) Device {
	return Device{bus: bus, dc: dc, cs: cs, width: 320, height: 240}
}

// Configure sends the init list of the variant and sets inversion, the pixel
// format (RGB565) and the rotation. A hardware reset, if any, must be done by
// the caller before.
// ref: M5GFX src/lgfx/v1/panel/Panel_LCD.inl (Panel_LCD::init),
//
//	src/lgfx/v1/LGFXBase.inl (LGFX_Device::init_impl: invertDisplay, setColorDepth, setRotation)
func (d *Device) Configure(cfg Config) error {
	var list []byte
	switch cfg.Variant {
	case VariantC:
		list = initC
	case VariantE:
		list = initE
	default:
		return ErrVariant
	}
	d.variant = cfg.Variant

	d.dc.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d.dc.High()
	if d.cs != machine.NoPin {
		d.cs.Configure(machine.PinConfig{Mode: machine.PinOutput})
		d.cs.High()
	}

	for i := 0; i+1 < len(list); {
		cmd, n := list[i], list[i+1]
		if cmd == 0xFF && n == 0xFF {
			break
		}
		args := list[i+2 : i+2+int(n&0x7F)]
		d.Command(cmd, args...)
		if n&0x80 != 0 {
			time.Sleep(120 * time.Millisecond)
		}
		i += 2 + int(n&0x7F)
	}

	d.SetInvert(cfg.Invert)
	d.Command(COLMOD, 0x55) // RGB565
	d.rgb = cfg.RGB
	d.SetRotation(cfg.Rotation)
	return nil
}

// Variant returns the configured controller variant.
func (d *Device) Variant() Variant { return d.variant }

// Command sends a command with optional parameters.
func (d *Device) Command(cmd byte, args ...byte) {
	d.begin()
	d.cmdBuf[0] = cmd
	d.dc.Low()
	d.bus.Tx(d.cmdBuf[:1], nil)
	d.dc.High()
	if len(args) > 0 {
		d.bus.Tx(args, nil)
	}
	d.end()
}

func (d *Device) begin() {
	if d.cs != machine.NoPin {
		d.cs.Low()
	}
}

func (d *Device) end() {
	if d.cs != machine.NoPin {
		d.cs.High()
	}
}

// SetInvert turns display inversion on or off.
func (d *Device) SetInvert(on bool) {
	if on {
		d.Command(INVON)
	} else {
		d.Command(INVOFF)
	}
}

// SetRotation sets the controller rotation (0..7), see Config.Rotation.
// ref: M5GFX src/lgfx/v1/panel/Panel_LCD.inl (Panel_LCD::setRotation / update_madctl)
func (d *Device) SetRotation(r uint8) {
	r &= 7
	d.rotation = r
	if r&1 != 0 {
		d.width, d.height = 240, 320
	} else {
		d.width, d.height = 320, 240
	}
	m := madctlTable[r]
	if !d.rgb {
		m |= MAD_BGR
	}
	d.Command(MADCTL, m)
}

// Size returns the current size of the display.
func (d *Device) Size() (x, y int16) { return d.width, d.height }

// Display is a no-op (the controller has its own frame memory).
func (d *Device) Display() error { return nil }

// setWindow sets the address window and leaves the controller in RAMWR,
// inside an open transaction (CS low, D/C high).
func (d *Device) setWindow(x, y, w, h int16) {
	x1, y1 := x+w-1, y+h-1
	d.begin()
	d.dc.Low()
	d.cmdBuf[0] = CASET
	d.bus.Tx(d.cmdBuf[:1], nil)
	d.dc.High()
	d.cmdBuf[0], d.cmdBuf[1], d.cmdBuf[2], d.cmdBuf[3] = byte(x>>8), byte(x), byte(x1>>8), byte(x1)
	d.bus.Tx(d.cmdBuf[:4], nil)
	d.dc.Low()
	d.cmdBuf[0] = PASET
	d.bus.Tx(d.cmdBuf[:1], nil)
	d.dc.High()
	d.cmdBuf[0], d.cmdBuf[1], d.cmdBuf[2], d.cmdBuf[3] = byte(y>>8), byte(y), byte(y1>>8), byte(y1)
	d.bus.Tx(d.cmdBuf[:4], nil)
	d.dc.Low()
	d.cmdBuf[0] = RAMWR
	d.bus.Tx(d.cmdBuf[:1], nil)
	d.dc.High()
}

// SetPixel draws one pixel.
func (d *Device) SetPixel(x, y int16, c color.RGBA) {
	if x < 0 || y < 0 || x >= d.width || y >= d.height {
		return
	}
	d.setWindow(x, y, 1, 1)
	v := RGBATo565(c)
	d.fillBuf[0], d.fillBuf[1] = byte(v>>8), byte(v)
	d.bus.Tx(d.fillBuf[:2], nil)
	d.end()
}

// FillRectangle fills a rectangle, clipped to the screen.
func (d *Device) FillRectangle(x, y, w, h int16, c color.RGBA) error {
	return d.FillRectangle565(x, y, w, h, RGBATo565(c))
}

// FillRectangle565 fills a rectangle with an RGB565 color, clipped to the screen.
func (d *Device) FillRectangle565(x, y, w, h int16, c uint16) error {
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > d.width {
		w = d.width - x
	}
	if y+h > d.height {
		h = d.height - y
	}
	if w <= 0 || h <= 0 {
		return nil
	}
	hi, lo := byte(c>>8), byte(c)
	for i := 0; i < len(d.fillBuf); i += 2 {
		d.fillBuf[i], d.fillBuf[i+1] = hi, lo
	}
	d.setWindow(x, y, w, h)
	n := int(w) * int(h) * 2
	for n > 0 {
		k := n
		if k > len(d.fillBuf) {
			k = len(d.fillBuf)
		}
		d.bus.Tx(d.fillBuf[:k], nil)
		n -= k
	}
	d.end()
	return nil
}

// FillScreen fills the whole screen.
func (d *Device) FillScreen(c color.RGBA) {
	d.FillRectangle(0, 0, d.width, d.height, c)
}

// DrawRGBBitmap8 draws big-endian RGB565 pixel data (2 bytes per pixel).
// The rectangle must lie inside the screen.
func (d *Device) DrawRGBBitmap8(x, y int16, data []byte, w, h int16) error {
	if x < 0 || y < 0 || w <= 0 || h <= 0 || x+w > d.width || y+h > d.height {
		return errors.New("ili9342: rectangle outside display area")
	}
	if len(data) < int(w)*int(h)*2 {
		return errors.New("ili9342: not enough data")
	}
	d.setWindow(x, y, w, h)
	d.bus.Tx(data[:int(w)*int(h)*2], nil)
	d.end()
	return nil
}

// Sleep puts the panel into (or out of) sleep mode.
func (d *Device) Sleep(sleep bool) {
	if sleep {
		d.Command(SLPIN)
		time.Sleep(5 * time.Millisecond)
	} else {
		d.Command(SLPOUT)
		time.Sleep(120 * time.Millisecond)
	}
}

// RGBATo565 converts a color to RGB565.
func RGBATo565(c color.RGBA) uint16 {
	return uint16(c.R&0xF8)<<8 | uint16(c.G&0xFC)<<3 | uint16(c.B)>>3
}
