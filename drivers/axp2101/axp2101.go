// Package axp2101 implements a driver for the X-Powers AXP2101 power
// management IC.
//
// Only the registers needed by M5Stack CoreS3 class boards are covered.
// The register usage follows M5Unified's AXP2101_Class, which is the most
// widely tested open implementation for this chip.
//
// ref: M5Unified src/utility/power/AXP2101_Class.inl / AXP2101_Class.hpp
// Datasheet: http://www.x-powers.com/en.php/Info/product_detail/article_id/95
package axp2101

import (
	"errors"

	"tinygo.org/x/drivers"
)

// Address is the fixed I2C address.
const Address = 0x34

// Registers.
// ref: M5Unified src/utility/power/AXP2101_Class.inl / .hpp (register usage and comments)
const (
	RegStatus1      = 0x00 // bit5: VBUS good, bit3: battery present
	RegStatus2      = 0x01 // bit6..5: 01 charge / 10 discharge / 00 standby
	RegChipID       = 0x03
	RegCommonConfig = 0x10 // bit0: software power off
	RegChargerCtl   = 0x18 // bit1: battery charge enable
	RegDCDCUVP      = 0x23 // bit4..0: DCDC1-5 under-voltage power-off enable
	RegPwronKey     = 0x27 // power key on/off timing
	RegADCEnable    = 0x30
	RegVBAT         = 0x34 // 14 bit, mV
	RegTS           = 0x36 // 14 bit, 0.5 mV
	RegVBUS         = 0x38 // 14 bit, mV
	RegVSYS         = 0x3A // 14 bit, mV
	RegIRQEn0       = 0x40
	RegIRQEn1       = 0x41
	RegIRQEn2       = 0x42
	RegIRQStatus0   = 0x48
	RegIRQStatus1   = 0x49 // bit3: power key short press, bit2: power key long press
	RegIRQStatus2   = 0x4A
	RegPreCharge    = 0x61
	RegChargeCur    = 0x62
	RegChargeVolt   = 0x64
	RegCHGLED       = 0x69
	RegLDOEnable0   = 0x90 // bit0..3: ALDO1-4, bit4,5: BLDO1,2, bit6: CPUSLDO, bit7: DLDO1
	RegLDOEnable1   = 0x91 // bit0: DLDO2
	RegALDO1        = 0x92 // ALDO1..4 = 0x92..0x95, BLDO1,2 = 0x96,0x97
	RegDLDO1        = 0x99
	RegBatteryLevel = 0xA4 // fuel gauge, %
)

// ChipID is the value of RegChipID.
// ref: M5Unified AXP2101_Class::begin (_init = (val == 0x4A))
const ChipID = 0x4A

// Power key bits of RegIRQStatus1.
// ref: M5Unified AXP2101_Class::getPekPress (readRegister8(0x49) & 0x0C),
//
//	M5Unified.inl BtnPWR update (2 = clicked, otherwise hold)
const (
	IRQ1PowerKeyShort = 1 << 3
	IRQ1PowerKeyLong  = 1 << 2
)

// ChargeStatus is the state of the charger.
type ChargeStatus int8

const (
	Discharging ChargeStatus = -1
	Standby     ChargeStatus = 0
	Charging    ChargeStatus = 1
)

var ErrInvalidID = errors.New("axp2101: unexpected chip id")

// adcInvalidMin: a 14 bit channel reads near full scale when nothing is
// measured on it.
// ref: M5Unified AXP2101_Class.inl (adc_invalid_min = 0x3FFF - 32)
const adcInvalidMin = 0x3FFF - 32

// Device is an AXP2101 on an I2C bus.
type Device struct {
	bus     drivers.I2C
	Address uint16
	buf     [3]byte
}

// New returns a Device for the given bus. Call Configure before use.
func New(bus drivers.I2C) Device {
	return Device{bus: bus, Address: Address}
}

// Configure checks the chip ID. It does not change any register: the rail
// setup is board specific.
func (d *Device) Configure() error {
	id, err := d.ReadRegister(RegChipID)
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

// read14 reads a 14 bit ADC value (big endian, upper 2 bits unused).
// ref: M5Unified AXP2101_Class::readRegister14
func (d *Device) read14(reg uint8) (uint16, error) {
	d.buf[0] = reg
	err := d.bus.Tx(d.Address, d.buf[:1], d.buf[1:3])
	return uint16(d.buf[1]&0x3F)<<8 | uint16(d.buf[2]), err
}

// SetALDOVoltage sets ALDO1..4 (n = 1..4) to mV (500..3500, 100 mV/step) and
// enables it. mV < 500 disables the LDO.
// ref: M5Unified AXP2101_Class::_set_LDO
func (d *Device) SetALDOVoltage(n uint8, mV int) error {
	if n < 1 || n > 4 {
		return errors.New("axp2101: invalid ALDO")
	}
	return d.setLDO(n-1, mV)
}

// SetBLDOVoltage sets BLDO1..2 (n = 1..2), same encoding as ALDO.
func (d *Device) SetBLDOVoltage(n uint8, mV int) error {
	if n < 1 || n > 2 {
		return errors.New("axp2101: invalid BLDO")
	}
	return d.setLDO(n+3, mV)
}

func (d *Device) setLDO(num uint8, mV int) error {
	mV -= 500
	val := 0
	if mV > 0 {
		val = mV / 100
		if val > 0x1E {
			val = 0x1E
		}
	}
	if err := d.WriteRegister(RegALDO1+num, uint8(val)); err != nil {
		return err
	}
	if mV < 0 {
		return d.ClearBits(RegLDOEnable0, 1<<num)
	}
	return d.SetBits(RegLDOEnable0, 1<<num)
}

// SetDLDO1Voltage sets DLDO1 to mV (500..3300, 100 mV/step) and enables it.
// mV < 500 disables it.
// ref: M5Unified AXP2101_Class::_set_DLDO (num = 0: max 0x1C, 100 mV/step, enable = reg 0x90 bit7)
func (d *Device) SetDLDO1Voltage(mV int) error {
	mV -= 500
	val := 0
	if mV > 0 {
		val = mV / 100
		if val > 0x1C {
			val = 0x1C
		}
	}
	if err := d.WriteRegister(RegDLDO1, uint8(val)); err != nil {
		return err
	}
	if mV < 0 {
		return d.ClearBits(RegLDOEnable0, 0x80)
	}
	return d.SetBits(RegLDOEnable0, 0x80)
}

// IsVBUSGood reports whether a valid VBUS (USB) supply is present.
// ref: M5Unified AXP2101_Class::isVBUS (reg 0x00 & 0x20)
func (d *Device) IsVBUSGood() (bool, error) {
	v, err := d.ReadRegister(RegStatus1)
	return v&0x20 != 0, err
}

// IsBatteryPresent reports whether a battery is connected.
// ref: M5Unified AXP2101_Class::getBatState (reg 0x00 & 0x08)
func (d *Device) IsBatteryPresent() (bool, error) {
	v, err := d.ReadRegister(RegStatus1)
	return v&0x08 != 0, err
}

// ChargeStatus returns the charger state.
// ref: M5Unified AXP2101_Class::getChargeStatus ((reg 0x01 >> 5) & 3: 1 = charge, 2 = discharge, 0 = standby)
func (d *Device) ChargeStatus() (ChargeStatus, error) {
	v, err := d.ReadRegister(RegStatus2)
	switch (v >> 5) & 3 {
	case 1:
		return Charging, err
	case 2:
		return Discharging, err
	}
	return Standby, err
}

// BatteryVoltage returns the battery voltage in mV.
// ref: M5Unified AXP2101_Class::getBatteryVoltage (readRegister14(0x34) / 1000.0f V)
func (d *Device) BatteryVoltage() (uint16, error) {
	return d.read14(RegVBAT)
}

// VBUSVoltage returns the VBUS voltage in mV, or 0 when VBUS is absent.
// ref: M5Unified AXP2101_Class::getVBUSVoltage
func (d *Device) VBUSVoltage() (uint16, error) {
	good, err := d.IsVBUSGood()
	if err != nil || !good {
		return 0, err
	}
	v, err := d.read14(RegVBUS)
	if v >= adcInvalidMin {
		return 0, err
	}
	return v, err
}

// BatteryLevel returns the fuel gauge estimate in percent.
// ref: M5Unified AXP2101_Class::getBatteryLevel (readRegister8(0xA4))
func (d *Device) BatteryLevel() (uint8, error) {
	return d.ReadRegister(RegBatteryLevel)
}

// SetBatteryCharge enables or disables battery charging.
// ref: M5Unified AXP2101_Class::setBatteryCharge (reg 0x18 bit1)
func (d *Device) SetBatteryCharge(enable bool) error {
	if enable {
		return d.SetBits(RegChargerCtl, 0x02)
	}
	return d.ClearBits(RegChargerCtl, 0x02)
}

// SetChargeCurrent sets the constant charge current. Valid steps are 100..200 mA
// in 25 mA steps and 300..1000 mA in 100 mA steps; the value is rounded down,
// with 100 mA as the minimum.
// ref: M5Unified AXP2101_Class::setChargeCurrent (table 125..1000 mA, code = index + 4)
func (d *Device) SetChargeCurrent(mA uint16) error {
	var code uint8
	switch {
	case mA > 1000:
		code = 16
	case mA >= 300:
		code = 9 + uint8((mA-300)/100)
	case mA >= 200:
		code = 8
	case mA >= 100:
		code = uint8(mA / 25) // 100:4, 125:5, 150:6, 175:7
	default:
		code = 4
	}
	return d.WriteRegister(RegChargeCur, code)
}

// ReadPowerKey reads and clears the power key IRQ flags.
// ref: M5Unified AXP2101_Class::getPekPress
func (d *Device) ReadPowerKey() (short, long bool, err error) {
	v, err := d.ReadRegister(RegIRQStatus1)
	if err != nil {
		return false, false, err
	}
	v &= IRQ1PowerKeyShort | IRQ1PowerKeyLong
	if v != 0 {
		err = d.WriteRegister(RegIRQStatus1, v) // write 1 to clear
	}
	return v&IRQ1PowerKeyShort != 0, v&IRQ1PowerKeyLong != 0, err
}

// ClearIRQStatus clears all IRQ status bits.
// ref: M5Unified AXP2101_Class::clearIRQStatuses
func (d *Device) ClearIRQStatus() error {
	for i := uint8(0); i < 3; i++ {
		if err := d.WriteRegister(RegIRQStatus0+i, 0xFF); err != nil {
			return err
		}
	}
	return nil
}

// PowerOff switches the whole system off (software power off).
// ref: M5Unified AXP2101_Class::powerOff (bitOn(0x10, 0x01))
func (d *Device) PowerOff() error {
	return d.SetBits(RegCommonConfig, 0x01)
}
