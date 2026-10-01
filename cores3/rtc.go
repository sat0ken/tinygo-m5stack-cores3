package cores3

import (
	"runtime"
	"time"

	"esp32-s3-lite/drivers/bm8563"
)

// RTC is the BM8563, ready after InitRTC.
var RTC bm8563.Device

// InitRTC prepares the RTC.
func InitRTC() error {
	RTC = bm8563.New(I2C)
	return RTC.Configure()
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
