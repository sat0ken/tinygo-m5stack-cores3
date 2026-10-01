package cores3

// PowerButton is BtnPWR. The key is wired to the AXP2101 PWRON pin only, so it
// is read from the AXP2101 IRQ status (short press / long press), which gives
// clicked / hold events but no pressed state.
// ref: M5Unified src/M5Unified.inl (BtnPWR update: Power.getKeyState(): 2 = clicked, otherwise hold)
// ref: M5Unified src/utility/power/AXP2101_Class.inl (getPekPress)
type PowerButton struct {
	lastMsec uint32
	clicked  bool
	hold     bool
}

// btnPWRMinUpdateMsec: ref: M5Unified src/M5Unified.hpp (BTNPWR_MIN_UPDATE_MSEC = 4)
const btnPWRMinUpdateMsec = 4

// BtnPWR is the power button. Init must have been called before use.
var BtnPWR PowerButton

// Update polls the AXP2101. The Was* results hold until the next Update.
func (b *PowerButton) Update(msec uint32) {
	// M5Unified keeps reading while the previous state was an event, so that
	// the event lasts for exactly one update.
	if msec-b.lastMsec < btnPWRMinUpdateMsec && !b.clicked && !b.hold {
		return
	}
	b.lastMsec = msec
	short, long, err := PMIC.ReadPowerKey()
	if err != nil {
		short, long = false, false
	}
	b.clicked = short && !long
	b.hold = long
}

// WasClicked reports a short press seen by the last Update.
func (b *PowerButton) WasClicked() bool { return b.clicked }

// WasHold reports a long press seen by the last Update.
func (b *PowerButton) WasHold() bool { return b.hold }
