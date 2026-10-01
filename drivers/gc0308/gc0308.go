// Package gc0308 implements a minimal driver for the GalaxyCore GC0308 VGA
// camera sensor (SCCB = I2C register access): start-up, RGB565 output and
// frame size by subsampling.
//
// ref: espressif/esp32-camera sensors/gc0308.c (reset, set_pixformat,
// set_framesize with CONFIG_GC_SENSOR_SUBSAMPLE_MODE, the Kconfig default)
// ref: espressif/esp32-camera sensors/private_include/gc0308_regs.h
package gc0308

import (
	"errors"
	"time"

	"tinygo.org/x/drivers"
)

// Address is the SCCB address.
const Address = 0x21

// ChipID is the value of register 0x00 on page 0.
// ref: M5GFX src/M5GFX.cpp (chk_gc.value() == 0x9b)
const ChipID = 0x9b

// Registers. ref: gc0308_regs.h
const (
	regChipID      = 0x00
	regRowStartH   = 0x05
	regRowStartL   = 0x06
	regColStartH   = 0x07
	regColStartL   = 0x08
	regWinHeightH  = 0x09
	regWinHeightL  = 0x0a
	regWinWidthH   = 0x0b
	regWinWidthL   = 0x0c
	regOutputFmt   = 0x24
	regSubsampleEn = 0x53 // page 1, bit7
	regSubMode     = 0x54 // page 1
	regSubEn2      = 0x55 // page 1, bit0
	regSubY0       = 0x56
	regSubY1       = 0x57
	regSubUV0      = 0x58
	regSubUV1      = 0x59
	regPage        = 0xfe // RESET_RELATED: bit7 software reset, bit0..1 page
)

var ErrInvalidID = errors.New("gc0308: unexpected chip id")

// Device is a GC0308 on an I2C (SCCB) bus.
type Device struct {
	bus     drivers.I2C
	Address uint16
	buf     [2]byte
}

// New returns a Device for the given bus.
func New(bus drivers.I2C) Device {
	return Device{bus: bus, Address: Address}
}

// ReadRegister reads a register of the current page.
func (d *Device) ReadRegister(reg uint8) (uint8, error) {
	d.buf[0] = reg
	err := d.bus.Tx(d.Address, d.buf[:1], d.buf[1:2])
	return d.buf[1], err
}

// WriteRegister writes a register of the current page.
func (d *Device) WriteRegister(reg, value uint8) error {
	d.buf[0], d.buf[1] = reg, value
	return d.bus.Tx(d.Address, d.buf[:2], nil)
}

// setBits: new = (old &^ (mask << offset)) | ((value & mask) << offset).
// ref: gc0308.c set_reg_bits
func (d *Device) setBits(reg, offset, mask, value uint8) error {
	old, err := d.ReadRegister(reg)
	if err != nil {
		return err
	}
	return d.WriteRegister(reg, old&^(mask<<offset)|(value&mask)<<offset)
}

// Configure checks the chip ID, resets the sensor, loads the default
// registers and sets RGB565 output at 320x240 (QVGA).
func (d *Device) Configure() error {
	if err := d.WriteRegister(regPage, 0x00); err != nil {
		return err
	}
	id, err := d.ReadRegister(regChipID)
	if err != nil {
		return err
	}
	if id != ChipID {
		return ErrInvalidID
	}
	// ref: gc0308.c reset(): software reset, 80 ms, default regs, 80 ms, page 0
	if err := d.WriteRegister(regPage, 0xf0); err != nil {
		return err
	}
	time.Sleep(80 * time.Millisecond)
	for _, r := range defaultRegs {
		if r[0] == 0xff { // REG_DLY
			time.Sleep(time.Duration(r[1]) * time.Millisecond)
			continue
		}
		if err := d.WriteRegister(r[0], r[1]); err != nil {
			return err
		}
	}
	time.Sleep(80 * time.Millisecond)
	if err := d.WriteRegister(regPage, 0x00); err != nil {
		return err
	}
	// ref: gc0308.c set_pixformat(PIXFORMAT_RGB565): page 0, OUTPUT_FMT[3:0] = 6
	if err := d.setBits(regOutputFmt, 0, 0x0f, 6); err != nil {
		return err
	}
	return d.setQVGA()
}

// setQVGA selects 320x240 by 1/2 subsampling of the full 640x480 window.
// ref: gc0308.c set_framesize (subsample table entry {210, 420, 0x22, 0, 0, 0, 0}:
// win 640x480, row_s = col_s = 0, window registers = size + 8)
func (d *Device) setQVGA() error {
	const winW, winH = 640, 480
	steps := [...][2]uint8{
		{regPage, 0x00},
		{regRowStartH, 0}, {regRowStartL, 0},
		{regColStartH, 0}, {regColStartL, 0},
		{regWinHeightH, uint8((winH + 8) >> 8)}, {regWinHeightL, uint8((winH + 8) & 0xff)},
		{regWinWidthH, uint8((winW + 8) >> 8)}, {regWinWidthL, uint8((winW + 8) & 0xff)},
		{regPage, 0x01},
	}
	for _, s := range steps {
		if err := d.WriteRegister(s[0], s[1]); err != nil {
			return err
		}
	}
	if err := d.setBits(regSubsampleEn, 7, 0x01, 1); err != nil {
		return err
	}
	if err := d.setBits(regSubEn2, 0, 0x01, 1); err != nil {
		return err
	}
	steps2 := [...][2]uint8{
		{regSubMode, 0x22},
		{regSubY0, 0x00}, {regSubY1, 0x00},
		{regSubUV0, 0x00}, {regSubUV1, 0x00},
		{regPage, 0x00},
	}
	for _, s := range steps2 {
		if err := d.WriteRegister(s[0], s[1]); err != nil {
			return err
		}
	}
	return nil
}
