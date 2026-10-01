// Package ltr553 implements a driver for the Lite-On LTR-553ALS-WA ambient
// light (ALS) and proximity (PS) sensor.
//
// The register usage and the configuration follow M5Stack's LTR5XX library
// (LTR55X branch), which the CoreS3 tutorial uses.
//
// ref: M5CoreS3 src/utility/LTR5XX.h / LTR5XX.cpp
// Datasheet: https://optoelectronics.liteon.com/upload/download/DS86-2013-0003/LTR-553ALS-WA_DS_V1.0.pdf
package ltr553

import (
	"tinygo.org/x/drivers"
)

// Address is the fixed I2C address.
const Address = 0x23

// Registers (LTR-55x).
// ref: M5CoreS3 LTR5XX.cpp (#ifdef LTR55X register map)
const (
	RegALSControl   = 0x80 // bit4..2 gain, bit0 mode
	RegPSControl    = 0x81 // bit1 mode
	RegPSLED        = 0x82 // bit7..5 pulse freq, bit4..3 duty, bit2..0 peak current
	RegPSNPulses    = 0x83 // bit3..0
	RegPSMeasRate   = 0x84 // bit3..0
	RegALSMeasRate  = 0x85 // bit5..3 integration time, bit2..0 measurement rate
	RegPartID       = 0x86
	RegManufacID    = 0x87
	RegALSDataCh1_0 = 0x88 // CH1 low, CH1 high, CH0 low, CH0 high
	RegStatus       = 0x8C
	RegPSData0      = 0x8D // low, then high (bit2..0)
)

// Settings. ref: M5CoreS3 LTR5XX.h (LTR55X operating parameters)
const (
	ALSGain1X  = 0x00
	ALSGain2X  = 0x01
	ALSGain4X  = 0x02
	ALSGain8X  = 0x03
	ALSGain48X = 0x06
	ALSGain96X = 0x07

	ALSIntegration50ms  = 0x01
	ALSIntegration100ms = 0x00
	ALSIntegration150ms = 0x04
	ALSIntegration200ms = 0x02
	ALSIntegration250ms = 0x05
	ALSIntegration300ms = 0x06
	ALSIntegration350ms = 0x07
	ALSIntegration400ms = 0x03

	ALSMeasRate50ms   = 0x00
	ALSMeasRate100ms  = 0x01
	ALSMeasRate200ms  = 0x02
	ALSMeasRate500ms  = 0x03
	ALSMeasRate1000ms = 0x04
	ALSMeasRate2000ms = 0x05

	LEDDuty25  = 0x00
	LEDDuty50  = 0x01
	LEDDuty75  = 0x02
	LEDDuty100 = 0x03

	PSMeasRate10ms   = 0x08
	PSMeasRate50ms   = 0x00
	PSMeasRate70ms   = 0x01
	PSMeasRate100ms  = 0x02
	PSMeasRate200ms  = 0x03
	PSMeasRate500ms  = 0x04
	PSMeasRate1000ms = 0x05
	PSMeasRate2000ms = 0x06

	LEDPulse30kHz  = 0x00
	LEDPulse40kHz  = 0x01
	LEDPulse50kHz  = 0x02
	LEDPulse60kHz  = 0x03
	LEDPulse70kHz  = 0x04
	LEDPulse80kHz  = 0x05
	LEDPulse90kHz  = 0x06
	LEDPulse100kHz = 0x07

	LEDPeak5mA   = 0x00
	LEDPeak10mA  = 0x01
	LEDPeak20mA  = 0x02
	LEDPeak50mA  = 0x03
	LEDPeak100mA = 0x04
)

// Config is the basic configuration (Ltr5xx_Init_Basic_Para).
type Config struct {
	PSLEDPulseFreq     uint8
	PSLEDDutyCycle     uint8
	PSMeasurementRate  uint8
	PSLEDPeakCurrent   uint8
	PSLEDNPulses       uint8
	ALSGain            uint8
	ALSIntegrationTime uint8
	ALSMeasurementRate uint8
}

// DefaultConfig is LTR5XX_BASE_PARA_CONFIG_DEFAULT.
// ref: M5CoreS3 LTR5XX.h
var DefaultConfig = Config{
	PSLEDPulseFreq:     LEDPulse60kHz,
	PSLEDDutyCycle:     LEDDuty100,
	PSMeasurementRate:  PSMeasRate100ms,
	PSLEDPeakCurrent:   LEDPeak100mA,
	PSLEDNPulses:       1,
	ALSGain:            ALSGain1X,
	ALSIntegrationTime: ALSIntegration100ms,
	ALSMeasurementRate: ALSMeasRate500ms,
}

// Device is an LTR-553 on an I2C bus.
type Device struct {
	bus     drivers.I2C
	Address uint16
	buf     [5]byte
}

// New returns a Device for the given bus.
func New(bus drivers.I2C) Device {
	return Device{bus: bus, Address: Address}
}

// Configure puts PS and ALS in standby and writes cfg, in the same order as
// LTR5XX::begin(Ltr5xx_Init_Basic_Para*). Enable the sensors afterwards with
// SetPSActive / SetALSActive.
func (d *Device) Configure(cfg Config) error {
	steps := [...]struct{ reg, mask, val uint8 }{
		{RegPSControl, 0x02, 0},                               // PS standby
		{RegALSControl, 0x01, 0},                              // ALS standby
		{RegPSLED, 0xE0, cfg.PSLEDPulseFreq << 5},             // LED pulse freq
		{RegPSLED, 0x18, cfg.PSLEDDutyCycle << 3},             // LED duty cycle
		{RegPSMeasRate, 0xFF, cfg.PSMeasurementRate & 0x0F},   // write, not RMW
		{RegPSLED, 0x07, cfg.PSLEDPeakCurrent},                // LED peak current
		{RegPSNPulses, 0xFF, cfg.PSLEDNPulses & 0x0F},         // write, not RMW
		{RegALSControl, 0x1C, cfg.ALSGain << 2},               // gain
		{RegALSMeasRate, 0x38, cfg.ALSIntegrationTime << 3},   // integration time
		{RegALSMeasRate, 0x07, cfg.ALSMeasurementRate & 0x07}, // measurement rate
	}
	for _, s := range steps {
		if err := d.update(s.reg, s.mask, s.val); err != nil {
			return err
		}
	}
	return nil
}

// update: new = (old &^ mask) | (val & mask); mask 0xFF writes without reading.
func (d *Device) update(reg, mask, val uint8) error {
	v := val
	if mask != 0xFF {
		old, err := d.ReadRegister(reg)
		if err != nil {
			return err
		}
		v = old&^mask | val&mask
	}
	return d.WriteRegister(reg, v)
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

// SetPSActive switches the proximity sensor between active and standby.
// ref: LTR5XX::setPsMode (PS_CONTR bit1)
func (d *Device) SetPSActive(active bool) error {
	var v uint8
	if active {
		v = 0x02
	}
	return d.update(RegPSControl, 0x02, v)
}

// SetALSActive switches the ambient light sensor between active and standby.
// ref: LTR5XX::setAlsMode (ALS_CONTR bit0)
func (d *Device) SetALSActive(active bool) error {
	var v uint8
	if active {
		v = 0x01
	}
	return d.update(RegALSControl, 0x01, v)
}

// PS returns the 11 bit proximity count (larger = closer).
// ref: LTR5XX::getPsValue
func (d *Device) PS() (uint16, error) {
	d.buf[0] = RegPSData0
	err := d.bus.Tx(d.Address, d.buf[:1], d.buf[1:3])
	return uint16(d.buf[2]&0x07)<<8 | uint16(d.buf[1]), err
}

// ALS returns the average of the two ALS channels, (CH0 + CH1) / 2, as the
// M5 library does.
// ref: LTR5XX::getAlsValue (LTR55X: (ch1_value + ch0_value) >> 1)
func (d *Device) ALS() (uint16, error) {
	ch0, ch1, err := d.ALSChannels()
	return uint16((uint32(ch0) + uint32(ch1)) >> 1), err
}

// ALSChannels returns the raw CH0 (visible + IR) and CH1 (IR) counts.
func (d *Device) ALSChannels() (ch0, ch1 uint16, err error) {
	d.buf[0] = RegALSDataCh1_0
	err = d.bus.Tx(d.Address, d.buf[:1], d.buf[1:5])
	ch1 = uint16(d.buf[2])<<8 | uint16(d.buf[1])
	ch0 = uint16(d.buf[4])<<8 | uint16(d.buf[3])
	return ch0, ch1, err
}
