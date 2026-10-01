// Package es7210 implements a minimal driver for the Everest ES7210 4-channel
// audio ADC (I2S/TDM output).
//
// Only the start-up used by M5Stack CoreS3 (MIC1 + MIC2, 16 bit I2S, MCLK =
// 256 fs) is implemented.
//
// ref: M5Unified src/M5Unified.inl (es7210_write_reg, _microphone_enabled_cb_cores3)
package es7210

import "tinygo.org/x/drivers"

// Address is the CoreS3 I2C address (AD1=0, AD0=0).
const Address = 0x40

// RegResetCtl is RESET_CTL.
const RegResetCtl = 0x00

// Device is an ES7210 on an I2C bus.
type Device struct {
	bus     drivers.I2C
	Address uint16
	buf     [2]byte
}

// New returns a Device for the given bus.
func New(bus drivers.I2C) Device {
	return Device{bus: bus, Address: Address}
}

// WriteRegister writes one register.
func (d *Device) WriteRegister(reg, value uint8) error {
	d.buf[0], d.buf[1] = reg, value
	return d.bus.Tx(d.Address, d.buf[:2], nil)
}

// ReadRegister reads one register.
func (d *Device) ReadRegister(reg uint8) (uint8, error) {
	d.buf[0] = reg
	err := d.bus.Tx(d.Address, d.buf[:1], d.buf[1:2])
	return d.buf[1], err
}

// enableSeq is the CoreS3 start-up sequence.
// ref: M5Unified _microphone_enabled_cb_cores3 (enabled branch)
var enableSeq = [...][2]uint8{
	{0x00, 0x41}, // RESET_CTL
	{0x01, 0x1f}, // CLK_ON_OFF
	{0x06, 0x00}, // DIGITAL_PDN
	{0x07, 0x20}, // ADC_OSR
	{0x08, 0x10}, // MODE_CFG
	{0x09, 0x30}, // TCT0_CHPINI
	{0x0A, 0x30}, // TCT1_CHPINI
	{0x20, 0x0a}, // ADC34_HPF2
	{0x21, 0x2a}, // ADC34_HPF1
	{0x22, 0x0a}, // ADC12_HPF2
	{0x23, 0x2a}, // ADC12_HPF1
	{0x02, 0xC1},
	{0x04, 0x01},
	{0x05, 0x00},
	{0x11, 0x60},
	{0x40, 0x42}, // ANALOG_SYS
	{0x41, 0x70}, // MICBIAS12
	{0x42, 0x70}, // MICBIAS34
	{0x43, 0x1B}, // MIC1_GAIN
	{0x44, 0x1B}, // MIC2_GAIN
	{0x45, 0x00}, // MIC3_GAIN
	{0x46, 0x00}, // MIC4_GAIN
	{0x47, 0x00}, // MIC1_LP
	{0x48, 0x00}, // MIC2_LP
	{0x49, 0x00}, // MIC3_LP
	{0x4A, 0x00}, // MIC4_LP
	{0x4B, 0x00}, // MIC12_PDN
	{0x4C, 0xFF}, // MIC34_PDN
	{0x01, 0x14}, // CLK_ON_OFF
}

// Enable resets the chip and starts MIC1/MIC2 conversion. MCLK should be
// running.
// ref: M5Unified _microphone_enabled_cb_cores3 (RESET_CTL = 0xFF first, then the list)
func (d *Device) Enable() error {
	if err := d.WriteRegister(RegResetCtl, 0xFF); err != nil {
		return err
	}
	for _, s := range enableSeq {
		if err := d.WriteRegister(s[0], s[1]); err != nil {
			return err
		}
	}
	return nil
}

// Disable puts the chip in reset.
// ref: M5Unified _microphone_enabled_cb_cores3 (RESET_CTL = 0xFF, disabled branch writes nothing else)
func (d *Device) Disable() error {
	return d.WriteRegister(RegResetCtl, 0xFF)
}
