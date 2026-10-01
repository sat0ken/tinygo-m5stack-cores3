package cores3

import (
	"runtime"
	"time"

	"esp32-s3-lite/drivers/bm8563"
)

// RTC is the BM8563, ready after InitRTC.
var RTC bm8563.Device

// InitRTC prepares the RTC and disables its interrupts, as M5.begin() does
// with the default cfg.disable_rtc_irq = true.
// ref: M5Unified src/M5Unified.inl (if (cfg.disable_rtc_irq) { M5.Rtc.disableIRQ(); })
func InitRTC() error {
	RTC = bm8563.New(I2C)
	if err := RTC.Configure(); err != nil {
		return err
	}
	return RTC.DisableIRQ()
}

// SyncSystemTime sets the ESP32 clock (time.Now) from the RTC (UTC).
func SyncSystemTime() (time.Time, error) {
	t, err := RTC.ReadTime()
	if err != nil {
		return t, err
	}
	runtime.AdjustTimeOffset(t.UnixNano() - time.Now().UnixNano())
	return t, nil
}
