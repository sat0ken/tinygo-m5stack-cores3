package cores3

import (
	"device/esp"
	"machine"
	"time"
)

// i2cBus wraps machine.I2C0 to work around two problems of the TinyGo 0.42
// ESP32-S3 I2C driver (machine_esp32xx_i2c.go), observed on a CoreS3:
//
//  1. Tx with a read part returns before the transaction has finished (before
//     TRANS_COMPLETE, i.e. before STOP). A transaction started right after it
//     is corrupted: it returns the previous byte without error, and the next
//     one fails with NACK. Wait for TRANS_COMPLETE after such a Tx. The driver
//     clears the interrupt bits on return, so when TRANS_COMPLETE came first
//     the bit is gone and the wait ends at its timeout.
//  2. After a NACK every following transaction fails until the controller is
//     configured again. Reconfigure after an error.
//
// See docs/notes.md.
type i2cBus struct {
	bus *machine.I2C
	reg *esp.I2C_Type
	cfg machine.I2CConfig
}

func (b *i2cBus) configure() error {
	return b.bus.Configure(b.cfg)
}

// Tx implements drivers.I2C.
func (b *i2cBus) Tx(addr uint16, w, r []byte) error {
	err := b.bus.Tx(addr, w, r)
	if err != nil {
		b.configure()
		return err
	}
	if len(r) > 0 {
		start := time.Now()
		for time.Since(start) < 2*time.Millisecond {
			if b.reg.INT_RAW.Get()&esp.I2C_INT_RAW_TRANS_COMPLETE_INT_RAW_Msk != 0 {
				break
			}
		}
	}
	return nil
}
