// Package aw9523b implements a driver for the Awinic AW9523B 16-bit I2C GPIO
// expander / LED driver.
//
// Only the GPIO part is implemented.
//
// Datasheet: https://www.awinic.com/en/productDetail/AW9523BTQR
package aw9523b

import (
	"errors"

	"tinygo.org/x/drivers"
)

// Address is the default I2C address (AD1=AD0=0).
const Address = 0x58

// Registers.
// ref: AW9523B datasheet "Register list"
// ref: M5GFX src/M5GFX.cpp (CoreS3: 0x02/0x03 output, 0x04/0x05 CONFIG_P0/P1, 0x10 ID, 0x11 GCR, 0x12/0x13 LEDMODE)
const (
	RegInputP0   = 0x00 // reading clears the P0 input-change interrupt
	RegInputP1   = 0x01 // reading clears the P1 input-change interrupt
	RegOutputP0  = 0x02
	RegOutputP1  = 0x03
	RegConfigP0  = 0x04 // 1 = input, 0 = output
	RegConfigP1  = 0x05
	RegIntP0     = 0x06 // 1 = interrupt disabled, 0 = enabled
	RegIntP1     = 0x07
	RegID        = 0x10
	RegGCR       = 0x11 // bit4: P0 output mode (1 = push-pull, 0 = open-drain)
	RegLEDModeP0 = 0x12 // 1 = GPIO mode, 0 = LED (current) mode
	RegLEDModeP1 = 0x13
	RegSoftReset = 0x7F
)

// ChipID is the value of RegID.
// ref: M5GFX src/M5GFX.cpp (chk_aw.value() == 0x23)
const ChipID = 0x23

// Port selects one of the two 8-bit ports.
type Port uint8

const (
	Port0 Port = 0
	Port1 Port = 1
)

var ErrInvalidID = errors.New("aw9523b: unexpected chip id")

// Device is an AW9523B on an I2C bus.
type Device struct {
	bus     drivers.I2C
	Address uint16
	buf     [2]byte
}

// New returns a Device for the given bus. Call Configure before use.
func New(bus drivers.I2C) Device {
	return Device{bus: bus, Address: Address}
}

// Configure checks that the chip answers with the expected ID. It does not
// change any register: the pin setup is board specific.
func (d *Device) Configure() error {
	id, err := d.ReadRegister(RegID)
	if err != nil {
		return err
	}
	if id != ChipID {
		return ErrInvalidID
	}
	return nil
}

// ReadRegister reads one register.
func (d *Device) ReadRegister(reg uint8) (uint8, error) {
	d.buf[0] = reg
	err := d.bus.Tx(d.Address, d.buf[:1], d.buf[1:2])
	return d.buf[1], err
}

// WriteRegister writes one register.
func (d *Device) WriteRegister(reg, value uint8) error {
	d.buf[0] = reg
	d.buf[1] = value
	return d.bus.Tx(d.Address, d.buf[:2], nil)
}

// UpdateRegister does a read-modify-write: new = (old &^ mask) | (value & mask).
func (d *Device) UpdateRegister(reg, mask, value uint8) error {
	old, err := d.ReadRegister(reg)
	if err != nil {
		return err
	}
	return d.WriteRegister(reg, (old&^mask)|(value&mask))
}

// SetBits sets the bits in mask.
func (d *Device) SetBits(reg, mask uint8) error { return d.UpdateRegister(reg, mask, mask) }

// ClearBits clears the bits in mask.
func (d *Device) ClearBits(reg, mask uint8) error { return d.UpdateRegister(reg, mask, 0) }

// Input returns the input level of a port. Reading also clears the
// input-change interrupt of that port.
func (d *Device) Input(p Port) (uint8, error) { return d.ReadRegister(RegInputP0 + uint8(p)) }

// Output returns the output latch of a port.
func (d *Device) Output(p Port) (uint8, error) { return d.ReadRegister(RegOutputP0 + uint8(p)) }

// SetOutput writes the whole output latch of a port.
func (d *Device) SetOutput(p Port, value uint8) error {
	return d.WriteRegister(RegOutputP0+uint8(p), value)
}

// SetPins drives the pins in mask high (level=true) or low, leaving the others.
func (d *Device) SetPins(p Port, mask uint8, level bool) error {
	if level {
		return d.SetBits(RegOutputP0+uint8(p), mask)
	}
	return d.ClearBits(RegOutputP0+uint8(p), mask)
}

// SetDirection writes the direction register of a port (1 = input, 0 = output).
func (d *Device) SetDirection(p Port, inputs uint8) error {
	return d.WriteRegister(RegConfigP0+uint8(p), inputs)
}

// SetInterruptMask writes the interrupt enable register of a port
// (1 = interrupt disabled, 0 = enabled).
func (d *Device) SetInterruptMask(p Port, disabled uint8) error {
	return d.WriteRegister(RegIntP0+uint8(p), disabled)
}
