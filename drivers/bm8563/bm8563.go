// Package bm8563 implements a driver for the BM8563 real-time clock, which is
// register compatible with the NXP PCF8563.
//
// tinygo.org/x/drivers/pcf8563 (v0.36.0) is not used because its ReadTime
// masks the month with 0x0F (October..December are read back wrong) and it
// stores the weekday as 1..7. This driver follows M5Unified's PCF8563_Class.
//
// ref: M5Unified src/utility/rtc/PCF8563_Class.inl
// Datasheet: https://www.nxp.com/docs/en/data-sheet/PCF8563.pdf
package bm8563

import (
	"errors"
	"time"

	"tinygo.org/x/drivers"
)

// Address is the fixed I2C address.
const Address = 0x51

// Registers.
// ref: PCF8563 datasheet "Register overview"
const (
	RegControl1   = 0x00
	RegControl2   = 0x01 // bit3 AF, bit2 TF, bit1 AIE, bit0 TIE
	RegSeconds    = 0x02 // bit7 VL
	RegMinutes    = 0x03
	RegHours      = 0x04
	RegDays       = 0x05
	RegWeekdays   = 0x06
	RegMonths     = 0x07 // bit7 century
	RegYears      = 0x08
	RegTimerCtrl  = 0x0E // bit7 TE, bit1..0 TD (source clock)
	RegTimerValue = 0x0F
)

// Control2 bits.
const (
	Ctrl2TIE = 1 << 0
	Ctrl2AIE = 1 << 1
	Ctrl2TF  = 1 << 2
	Ctrl2AF  = 1 << 3
)

var ErrInvalidTime = errors.New("bm8563: invalid time in registers")

// Device is a BM8563 on an I2C bus.
type Device struct {
	bus     drivers.I2C
	Address uint16
	buf     [8]byte
}

// New returns a Device for the given bus. Call Configure before use.
func New(bus drivers.I2C) Device {
	return Device{bus: bus, Address: Address}
}

// Configure initializes the control registers.
// ref: M5Unified PCF8563_Class::begin (0x00 = 0x00 twice, 0x0E = 0x03)
func (d *Device) Configure() error {
	d.WriteRegister(RegControl1, 0x00) // dummy write: the first access may fail
	if err := d.WriteRegister(RegControl1, 0x00); err != nil {
		return err
	}
	return d.WriteRegister(RegTimerCtrl, 0x03) // timer off, source 1/60 Hz
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

// VoltLow reports the VL flag: the clock integrity is not guaranteed (the
// oscillator stopped since the time was last set).
// ref: M5Unified PCF8563_Class::getVoltLow (readRegister8(0x02) & 0x80)
func (d *Device) VoltLow() (bool, error) {
	v, err := d.ReadRegister(RegSeconds)
	return v&0x80 != 0, err
}

// ReadTime returns the date and time stored in the RTC, in UTC.
// ref: M5Unified PCF8563_Class::getDateTime
func (d *Device) ReadTime() (time.Time, error) {
	d.buf[0] = RegSeconds
	if err := d.bus.Tx(d.Address, d.buf[:1], d.buf[1:8]); err != nil {
		return time.Time{}, err
	}
	b := d.buf[1:8]
	sec, min, hour := b[0]&0x7F, b[1]&0x7F, b[2]&0x3F
	day, mon, yy := b[3]&0x3F, b[5]&0x1F, b[6]
	if !validBCD(sec) || !validBCD(min) || !validBCD(hour) || !validBCD(day) || !validBCD(mon) || !validBCD(yy) {
		return time.Time{}, ErrInvalidTime
	}
	year := 2000 + bcd2dec(yy)
	if b[5]&0x80 != 0 { // century bit: 19xx
		year -= 100
	}
	s, mi, h, dd, mo := bcd2dec(sec), bcd2dec(min), bcd2dec(hour), bcd2dec(day), bcd2dec(mon)
	if s > 59 || mi > 59 || h > 23 || dd < 1 || dd > 31 || mo < 1 || mo > 12 {
		return time.Time{}, ErrInvalidTime
	}
	return time.Date(year, time.Month(mo), dd, h, mi, s, 0, time.UTC), nil
}

// SetTime stores t (converted to UTC). Writing the seconds clears VL.
// ref: M5Unified PCF8563_Class::setDateTime (weekday 0..6, century bit for < 2000)
func (d *Device) SetTime(t time.Time) error {
	t = t.UTC()
	d.buf[0] = RegSeconds
	d.buf[1] = dec2bcd(t.Second())
	d.buf[2] = dec2bcd(t.Minute())
	d.buf[3] = dec2bcd(t.Hour())
	d.buf[4] = dec2bcd(t.Day())
	d.buf[5] = uint8(t.Weekday()) & 0x07
	d.buf[6] = dec2bcd(int(t.Month()))
	if t.Year() < 2000 {
		d.buf[6] |= 0x80
	}
	d.buf[7] = dec2bcd(t.Year() % 100)
	return d.bus.Tx(d.Address, d.buf[:8], nil)
}

// DisableIRQ disables the alarm (all alarm fields) and the timer.
// ref: M5Unified PCF8563_Class::disableIRQ (0x09..0x0C = 0x80, 0x0E = 0)
func (d *Device) DisableIRQ() error {
	d.buf[0] = 0x09
	d.buf[1], d.buf[2], d.buf[3], d.buf[4] = 0x80, 0x80, 0x80, 0x80
	if err := d.bus.Tx(d.Address, d.buf[:5], nil); err != nil {
		return err
	}
	return d.WriteRegister(RegTimerCtrl, 0)
}

// ClearIRQ clears the alarm and timer flags (AF, TF).
// ref: M5Unified PCF8563_Class::clearIRQ (bitOff(0x01, 0x0C))
func (d *Device) ClearIRQ() error {
	v, err := d.ReadRegister(RegControl2)
	if err != nil {
		return err
	}
	return d.WriteRegister(RegControl2, v&^(Ctrl2TF|Ctrl2AF))
}

// SetTimer starts the countdown timer with interrupt output (INT low when it
// expires) and returns the programmed duration. 0 disables the timer.
// Up to 255 s it counts seconds, above that minutes (max 255 min).
// ref: M5Unified PCF8563_Class::setTimerIRQ
func (d *Device) SetTimer(dur time.Duration) (time.Duration, error) {
	ctrl2, err := d.ReadRegister(RegControl2)
	if err != nil {
		return 0, err
	}
	ctrl2 &^= Ctrl2TF | Ctrl2AF // keep the flags cleared (write 0 clears)
	ms := int64(dur / time.Millisecond)
	after := ms / 1000
	if ms%1000 >= 500 {
		after++
	}
	if after <= 0 {
		if err := d.WriteRegister(RegControl2, ctrl2&^Ctrl2TIE); err != nil {
			return 0, err
		}
		return 0, d.WriteRegister(RegTimerCtrl, 0x03)
	}
	div := int64(1)
	typ := uint8(0x82) // TE=1, source 1 Hz
	if after < 270 {
		if after > 255 {
			after = 255
		}
	} else {
		div = 60
		after = (after + 30) / div
		if after > 255 {
			after = 255
		}
		typ = 0x83 // TE=1, source 1/60 Hz
	}
	if err := d.WriteRegister(RegTimerCtrl, typ); err != nil {
		return 0, err
	}
	if err := d.WriteRegister(RegTimerValue, uint8(after)); err != nil {
		return 0, err
	}
	// M5Unified: (reg_value | 0x01) & ~0x80 (TIE on)
	if err := d.WriteRegister(RegControl2, ctrl2|Ctrl2TIE); err != nil {
		return 0, err
	}
	return time.Duration(after*div) * time.Second, nil
}

func validBCD(v uint8) bool { return v&0x0F <= 9 && v>>4 <= 9 }

func bcd2dec(v uint8) int { return int(v>>4)*10 + int(v&0x0F) }

func dec2bcd(v int) uint8 { return uint8(v/10)<<4 | uint8(v%10) }
