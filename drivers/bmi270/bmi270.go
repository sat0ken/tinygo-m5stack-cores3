// Package bmi270 implements a driver for the Bosch BMI270 6-axis IMU,
// including access to a magnetometer on its auxiliary I2C interface.
//
// The BMI270 needs its 8 KB feature configuration (config.go) uploaded after
// every power-up.
//
// ref: Bosch BMI270_SensorAPI bmi2.c (bmi2_sec_init, bmi2_soft_reset, write_config_file, upload_file)
// ref: M5Unified src/utility/imu/BMI270_Class.inl (start-up, AUX setup, data read)
// Datasheet: https://www.bosch-sensortec.com/products/motion-sensors/imus/bmi270/
package bmi270

import (
	"errors"
	"time"

	"tinygo.org/x/drivers"
)

// I2C addresses (SDO low / high). The CoreS3 uses 0x69.
const (
	Address     = 0x68
	AddressHigh = 0x69
)

// ChipID is the value of RegChipID.
const ChipID = 0x24

// Registers.
// ref: M5Unified src/utility/imu/BMI270_Class.hpp, Bosch bmi2_defs.h
const (
	RegChipID         = 0x00
	RegErr            = 0x02
	RegStatus         = 0x03 // bit2: aux_man_op (AUX manual access busy)
	RegAuxXLSB        = 0x04 // AUX data (8 bytes), then ACC (6), GYR (6)
	RegAccXLSB        = 0x0C
	RegGyrXLSB        = 0x12
	RegIntStatus1     = 0x1D // bit7 acc drdy, bit6 gyr drdy, bit5 aux drdy
	RegInternalStatus = 0x21 // bit3..0 message: 1 = init ok
	RegTemperature0   = 0x22
	RegAccConf        = 0x40
	RegAccRange       = 0x41
	RegGyrConf        = 0x42
	RegGyrRange       = 0x43
	RegAuxDevID       = 0x4B
	RegAuxIfConf      = 0x4C
	RegAuxRdAddr      = 0x4D
	RegAuxWrAddr      = 0x4E
	RegAuxWrData      = 0x4F
	RegIntMapData     = 0x58
	RegInitCtrl       = 0x59
	RegInitAddr0      = 0x5B
	RegInitData       = 0x5E
	RegIfConf         = 0x6B
	RegPwrConf        = 0x7C
	RegPwrCtrl        = 0x7D // bit3 temp, bit2 acc, bit1 gyr, bit0 aux
	RegCmd            = 0x7E

	CmdSoftReset = 0xB6
)

// Data ready bits of RegIntStatus1 (returned by ReadRaw).
const (
	ReadyAccel = 0x80
	ReadyGyro  = 0x40
	ReadyAux   = 0x20
)

var (
	ErrInvalidID  = errors.New("bmi270: unexpected chip id")
	ErrConfigLoad = errors.New("bmi270: config file load failed")
)

// uploadChunk is the burst length used for the config upload (must be even).
const uploadChunk = 128

// RawData is one sample as stored in the registers 0x04..0x17.
type RawData struct {
	Aux   [4]int16 // aux sensor data (BMM150: X, Y, Z, RHALL)
	Accel [3]int16
	Gyro  [3]int16
}

// Device is a BMI270 on an I2C bus.
type Device struct {
	bus     drivers.I2C
	Address uint16
	buf     [1 + uploadChunk]byte
}

// New returns a Device for the given bus. Call Configure before use.
func New(bus drivers.I2C) Device {
	return Device{bus: bus, Address: Address}
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

// Configure resets the chip and loads the configuration. Afterwards the
// accelerometer and gyroscope are still off: enable them with RegPwrCtrl.
// The default ranges after reset are ±8 g and ±2000 dps.
func (d *Device) Configure() error {
	d.ReadRegister(RegChipID) // M5Unified reads WhoAmI twice (the first access may fail)
	id, err := d.ReadRegister(RegChipID)
	if err != nil {
		return err
	}
	if id != ChipID {
		return ErrInvalidID
	}

	// Soft reset. ref: Bosch bmi2_soft_reset (delay 2000 us),
	// M5Unified BMI270_Class::begin (poll PWR_CONF != 0, up to 16 ticks)
	if err := d.WriteRegister(RegCmd, CmdSoftReset); err != nil {
		return err
	}
	time.Sleep(2 * time.Millisecond)
	for i := 0; i < 16; i++ {
		if v, err := d.ReadRegister(RegPwrConf); err == nil && v != 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// Upload. ref: Bosch write_config_file (adv. power save off, INIT_CTRL=0,
	// upload_file chunks with INIT_ADDR = index/2, INIT_CTRL=1)
	if err := d.WriteRegister(RegPwrConf, 0x00); err != nil {
		return err
	}
	time.Sleep(time.Millisecond) // datasheet: 450 us after disabling adv. power save
	if err := d.WriteRegister(RegInitCtrl, 0x00); err != nil {
		return err
	}
	for index := 0; index < len(configFile); index += uploadChunk {
		if err := d.uploadChunk(index); err != nil {
			return err
		}
	}
	if err := d.WriteRegister(RegInitCtrl, 0x01); err != nil {
		return err
	}
	// ref: M5Unified BMI270_Class::begin (INT_MAP_DATA = 0xFF)
	if err := d.WriteRegister(RegIntMapData, 0xFF); err != nil {
		return err
	}

	// Wait for the load result. ref: Bosch bmi2_write_config_file
	// (INTERNAL_STATUS & 0x0F == 1 means success); datasheet: up to 20 ms.
	var status uint8
	for i := 0; i < 150; i++ {
		time.Sleep(time.Millisecond)
		status, err = d.ReadRegister(RegInternalStatus)
		if err == nil && status&0x0F == 0x01 {
			return nil
		}
	}
	return ErrConfigLoad
}

func (d *Device) uploadChunk(index int) error {
	// ref: Bosch upload_file / M5Unified BMI270_Class::_upload_file
	d.buf[0] = RegInitAddr0
	d.buf[1] = uint8((index / 2) & 0x0F)
	d.buf[2] = uint8((index / 2) >> 4)
	if err := d.bus.Tx(d.Address, d.buf[:3], nil); err != nil {
		return err
	}
	n := copy(d.buf[1:], configFile[index:])
	d.buf[0] = RegInitData
	return d.bus.Tx(d.Address, d.buf[:1+n], nil)
}

// InternalStatus returns RegInternalStatus (bit3..0: 1 = init ok).
func (d *Device) InternalStatus() (uint8, error) { return d.ReadRegister(RegInternalStatus) }

// AuxSetupMode enables the auxiliary I2C interface in manual mode for the
// sensor at addr (7 bit). The AUX sensor is disabled in PWR_CTRL meanwhile.
// ref: M5Unified BMI270_Class::auxSetupMode
func (d *Device) AuxSetupMode(addr uint8) error {
	steps := [...][2]uint8{
		{RegIfConf, 0x20},    // AUX I2C enable
		{RegPwrConf, 0x00},   // power save disabled
		{RegPwrCtrl, 0x0E},   // AUX sensor disable
		{RegAuxIfConf, 0x80}, // manual mode
		{RegAuxDevID, addr << 1},
	}
	for _, s := range steps {
		if err := d.WriteRegister(s[0], s[1]); err != nil {
			return err
		}
	}
	return nil
}

func (d *Device) waitAux() {
	// ref: M5Unified (retry 3, STATUS bit2 = aux_man_op)
	for retry := 0; retry < 3; retry++ {
		v, err := d.ReadRegister(RegStatus)
		if err == nil && v&0x04 == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

// AuxWriteRegister writes a register of the AUX sensor (manual mode).
// ref: M5Unified BMI270_Class::auxWriteRegister8
func (d *Device) AuxWriteRegister(reg, value uint8) error {
	if err := d.WriteRegister(RegAuxWrData, value); err != nil {
		return err
	}
	if err := d.WriteRegister(RegAuxWrAddr, reg); err != nil {
		return err
	}
	d.waitAux()
	return nil
}

// AuxReadRegister reads a register of the AUX sensor (manual mode).
// ref: M5Unified BMI270_Class::auxReadRegister8
func (d *Device) AuxReadRegister(reg uint8) (uint8, error) {
	if err := d.WriteRegister(RegAuxIfConf, 0x80); err != nil { // manual, burst length 1
		return 0, err
	}
	if err := d.WriteRegister(RegAuxRdAddr, reg); err != nil {
		return 0, err
	}
	d.waitAux()
	return d.ReadRegister(RegAuxXLSB)
}

// ReadRaw reads the data ready flags and, when any is set, the AUX, ACC and
// GYR data in one burst. Only the parts flagged in ready are updated.
// ref: M5Unified BMI270_Class::getImuRawData
func (d *Device) ReadRaw(data *RawData) (ready uint8, err error) {
	ready, err = d.ReadRegister(RegIntStatus1)
	if err != nil {
		return 0, err
	}
	ready &= ReadyAccel | ReadyGyro | ReadyAux
	if ready == 0 {
		return 0, nil
	}
	d.buf[0] = RegAuxXLSB
	if err = d.bus.Tx(d.Address, d.buf[:1], d.buf[1:21]); err != nil {
		return 0, err
	}
	b := d.buf[1:21]
	le := func(i int) int16 { return int16(uint16(b[i]) | uint16(b[i+1])<<8) }
	if ready&ReadyAux != 0 {
		for i := 0; i < 4; i++ {
			data.Aux[i] = le(i * 2)
		}
	}
	if ready&ReadyAccel != 0 {
		for i := 0; i < 3; i++ {
			data.Accel[i] = le(8 + i*2)
		}
	}
	if ready&ReadyGyro != 0 {
		for i := 0; i < 3; i++ {
			data.Gyro[i] = le(14 + i*2)
		}
	}
	return ready, nil
}
