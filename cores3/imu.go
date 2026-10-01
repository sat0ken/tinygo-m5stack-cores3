package cores3

import (
	"errors"

	"esp32-s3-lite/drivers/bmi270"
)

// BMM150 registers used through the BMI270 AUX interface.
// ref: M5Unified src/utility/imu/BMI270_Class.inl (BMI270_Class::begin, AUX part)
const (
	bmm150ChipIDReg = 0x40 // WhoAmI, 0x32
	bmm150ChipID    = 0x32
	bmm150DataXLSB  = 0x42
	bmm150PowerReg  = 0x4B
	bmm150OpModeReg = 0x4C
)

// Conversion factors (raw LSB -> unit).
// ref: M5Unified src/utility/imu/IMU_Base.hpp (imu_convert_param_t defaults: accel 8/32768 G, gyro 2000/32768 dps)
// ref: M5Unified BMI270_Class::getConvertParam (mag_res = 10 * 4912 / 32760)
const (
	accelRes = 8.0 / 32768.0
	gyroRes  = 2000.0 / 32768.0
	magRes   = 10.0 * 4912.0 / 32760.0
)

// IMUData is one converted sample, like M5Unified imu_data_t.
type IMUData struct {
	Accel [3]float32 // G
	Gyro  [3]float32 // deg/s
	Mag   [3]float32 // M5Unified units (raw * 10*4912/32760)
}

// IMUSensor is the BMI270 with the BMM150 behind it.
type IMUSensor struct {
	Dev    bmi270.Device
	HasMag bool
	raw    bmi270.RawData
	Data   IMUData
}

// IMU is ready after InitIMU.
var IMU IMUSensor

var errNoIMU = errors.New("cores3: BMI270 not found")

// InitIMU loads the BMI270 configuration and sets up the BMM150 on its AUX
// interface, as M5Unified does for the CoreS3.
// ref: M5Unified BMI270_Class::begin, IMU_Class::begin (CoreS3: BMI270 at 0x69)
func InitIMU() error {
	IMU.Dev = bmi270.New(I2C)
	IMU.Dev.Address = AddrBMI270
	if err := IMU.Dev.Configure(); err != nil {
		return err
	}
	d := &IMU.Dev
	if Verbose {
		st, _ := d.InternalStatus()
		print("cores3: BMI270 config loaded, INTERNAL_STATUS=0x")
		printHex8(st)
		println()
	}

	// AUX: BMM150 at 0x10.
	d.AuxSetupMode(AddrBMM150)
	d.AuxWriteRegister(bmm150PowerReg, 0x83) // soft reset + power on
	d.AuxReadRegister(bmm150ChipIDReg)       // dummy read, as M5Unified
	who, _ := d.AuxReadRegister(bmm150ChipIDReg)
	if Verbose {
		print("cores3: BMM150 WhoAmI=0x")
		printHex8(who)
		println(" (0x32 expected)")
	}
	pwr := uint8(0x0E) // temp | acc | gyr
	if who == bmm150ChipID {
		d.AuxWriteRegister(bmm150OpModeReg, 0x38)  // normal mode, ODR 30 Hz
		d.WriteRegister(bmi270.RegAuxIfConf, 0x4F) // FCU_WRITE_EN + manual burst length 8 + burst length 8
		d.WriteRegister(bmi270.RegAuxRdAddr, bmm150DataXLSB)
		pwr = 0x0F // + aux
		IMU.HasMag = true
	}
	return d.WriteRegister(bmi270.RegPwrCtrl, pwr)
}

// Update reads new data if available and converts it into IMU.Data.
// It reports whether anything was updated (M5.Imu.update()).
func (s *IMUSensor) Update() bool {
	ready, err := s.Dev.ReadRaw(&s.raw)
	if err != nil || ready == 0 {
		return false
	}
	r := &s.raw
	for i := 0; i < 3; i++ {
		s.Data.Accel[i] = float32(r.Accel[i]) * accelRes
		s.Data.Gyro[i] = float32(r.Gyro[i]) * gyroRes
	}
	if ready&bmi270.ReadyAux != 0 {
		// BMM150 data: X, Y are 13 bit (bit15..3), Z is 15 bit (bit15..1).
		// ref: M5Unified BMI270_Class::getImuRawData (x >> 2, y >> 2, z & 0xFFFE)
		mx := float32(r.Aux[0]>>2) * magRes
		my := float32(r.Aux[1]>>2) * magRes
		mz := float32(int16(uint16(r.Aux[2])&0xFFFE)) * magRes
		// CoreS3: magnetometer Y and Z axes are inverted.
		// ref: M5Unified IMU_Class::begin (board_M5StackCoreS3 && addr 0x69: axis_invert_y | axis_invert_z)
		s.Data.Mag = [3]float32{mx, -my, -mz}
	}
	return true
}
