// Package aw88298 implements a minimal driver for the Awinic AW88298 digital
// class-D speaker amplifier (I2S input, 16 bit registers, big endian on I2C).
//
// Only the start-up used by M5Stack CoreS3 is implemented.
//
// ref: M5Unified src/M5Unified.inl (aw88298_write_reg, _speaker_enabled_cb_cores3)
package aw88298

import "tinygo.org/x/drivers"

// Address is the CoreS3 I2C address.
const Address = 0x36

// Registers used. ref: M5Unified _speaker_enabled_cb_cores3 comments
const (
	RegSysCtrl  = 0x04 // I2SEN, AMPPD, PWDN
	RegSysCtrl2 = 0x05 // RMSE, HAGCE, HDCCE, HMUTE
	RegI2SCtrl  = 0x06 // sample rate / BCK mode
	RegHAGCCfg4 = 0x0C // volume
	RegBstCtrl2 = 0x61 // boost mode
)

// Device is an AW88298 on an I2C bus.
type Device struct {
	bus     drivers.I2C
	Address uint16
	buf     [3]byte
}

// New returns a Device for the given bus.
func New(bus drivers.I2C) Device {
	return Device{bus: bus, Address: Address}
}

// WriteRegister writes a 16 bit register (MSB first).
// ref: M5Unified aw88298_write_reg (value = __builtin_bswap16(value))
func (d *Device) WriteRegister(reg uint8, value uint16) error {
	d.buf[0], d.buf[1], d.buf[2] = reg, byte(value>>8), byte(value)
	return d.bus.Tx(d.Address, d.buf[:3], nil)
}

// ReadRegister reads a 16 bit register.
func (d *Device) ReadRegister(reg uint8) (uint16, error) {
	d.buf[0] = reg
	err := d.bus.Tx(d.Address, d.buf[:1], d.buf[1:3])
	return uint16(d.buf[1])<<8 | uint16(d.buf[2]), err
}

// Enable starts the amplifier for a 16 bit x 2 slot I2S stream at sampleRate.
// The I2S clocks should already be running.
// ref: M5Unified _speaker_enabled_cb_cores3 (enabled branch)
func (d *Device) Enable(sampleRate uint32) error {
	// Sample rate field of reg 0x06, chosen from (rate + 1102) / 2205.
	rateTbl := [...]uint32{4, 5, 6, 8, 10, 11, 15, 20, 22, 44}
	v := uint16(0)
	rate := (sampleRate + 1102) / 2205
	for int(v) < len(rateTbl) && rate > rateTbl[v] {
		v++
	}
	v |= 0x14C0 // I2SBCK=0 (BCK mode 16*2)
	steps := [...]struct {
		reg uint8
		val uint16
	}{
		{RegBstCtrl2, 0x0673}, // boost mode disabled
		{RegSysCtrl, 0x4040},  // I2SEN=1 AMPPD=0 PWDN=0
		{RegSysCtrl2, 0x0008}, // RMSE=0 HAGCE=0 HDCCE=0 HMUTE=0
		{RegI2SCtrl, v},
		{RegHAGCCfg4, 0x0064}, // volume setting (full volume)
	}
	for _, s := range steps {
		if err := d.WriteRegister(s.reg, s.val); err != nil {
			return err
		}
	}
	return nil
}

// Disable stops the I2S input of the amplifier.
// ref: M5Unified _speaker_enabled_cb_cores3 (disabled branch: 0x04 = 0x4000)
func (d *Device) Disable() error {
	return d.WriteRegister(RegSysCtrl, 0x4000)
}
