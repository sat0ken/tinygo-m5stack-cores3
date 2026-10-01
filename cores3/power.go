package cores3

import (
	"time"

	"esp32-s3-lite/drivers/ili9342"
)

// TimerSleep powers the board off and lets the RTC turn it on again after
// the given duration (M5.Power.timerSleep(seconds)). InitRTC must have been
// called. It does not return on success.
//
// On the CoreS3 the RTC INT is not wired to the ESP32 (it is shared with the
// AXP2101 IRQ line), so M5Unified does not deep-sleep: it arms the RTC timer
// and switches the AXP2101 off; the RTC interrupt powers the board up again.
// ref: M5Unified src/utility/Power_Class.inl (timerSleep(int), _timerSleep, _powerOff:
//
//	_rtcIntPin == GPIO_NUM_MAX on CoreS3 -> Axp2101.powerOff())
//
// ref: M5Unified src/utility/RTC_Class.hpp (setAlarmIRQ(int s) = setTimerIRQ(s * 1000))
func TimerSleep(d time.Duration) error {
	// M5.Display.sleep(): brightness 0, then SLPIN.
	// ref: M5GFX src/lgfx/v1/LGFXBase.hpp (LGFX_Device::sleep)
	SetBrightness(0)
	Display.Command(ili9342.SLPIN)
	time.Sleep(5 * time.Millisecond)

	if err := RTC.DisableIRQ(); err != nil {
		return err
	}
	if err := RTC.ClearIRQ(); err != nil {
		return err
	}
	if _, err := RTC.SetTimer(d); err != nil {
		return err
	}
	return PMIC.PowerOff()
}
