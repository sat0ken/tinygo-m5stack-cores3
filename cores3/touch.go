package cores3

import (
	"errors"
	"machine"

	"esp32-s3-lite/drivers/aw9523b"
)

// FT6336U registers (FocalTech FT5x06 family register map).
// ref: M5GFX src/lgfx/v1/touch/Touch_FT5x06.inl
const (
	ft5x06RegTDStatus = 0x02 // number of touch points, followed by P1_XH ...
	ft5x06RegCipher   = 0xA3
	ft5x06RegIntMode  = 0xA4 // 0x00 = INT polling mode
)

// TouchState is the touch state, bit compatible with M5Unified m5::touch_state_t.
// ref: M5Unified src/utility/Touch_Class.hpp
type TouchState uint8

const (
	TouchNone       TouchState = 0b0000
	TouchTouch      TouchState = 0b0001
	TouchTouchEnd   TouchState = 0b0010
	TouchTouchBegin TouchState = 0b0011
	TouchHold       TouchState = 0b0101
	TouchHoldEnd    TouchState = 0b0110
	TouchHoldBegin  TouchState = 0b0111
	TouchFlick      TouchState = 0b1001
	TouchFlickEnd   TouchState = 0b1010
	TouchFlickBegin TouchState = 0b1011
	TouchDrag       TouchState = 0b1101
	TouchDragEnd    TouchState = 0b1110
	TouchDragBegin  TouchState = 0b1111

	touchMaskTouch   TouchState = 0b0001
	touchMaskChange  TouchState = 0b0010
	touchMaskHolding TouchState = 0b0100
	touchMaskMoving  TouchState = 0b1000
)

// state names as printed by the M5 Touch tutorial (index = state value).
var touchStateNames = [16]string{
	"none", "touch", "touch_end", "touch_begin",
	"___", "hold", "hold_end", "hold_begin",
	"___", "flick", "flick_end", "flick_begin",
	"___", "drag", "drag_end", "drag_begin",
}

func (s TouchState) String() string { return touchStateNames[s&15] }

// TouchDetail is the state of one touch point, like M5Unified touch_detail_t.
type TouchDetail struct {
	X, Y         int16 // current point (updated while flicking/dragging, else the start point)
	PrevX, PrevY int16 // previous point
	BaseX, BaseY int16 // point where the touch / flick started
	BaseMsec     uint32
	State        TouchState
	ClickCount   uint8
	ID           uint8
}

func (d *TouchDetail) IsPressed() bool  { return d.State&touchMaskTouch != 0 }
func (d *TouchDetail) WasPressed() bool { return d.State == TouchTouchBegin }
func (d *TouchDetail) WasClicked() bool { return d.State == TouchTouchEnd }
func (d *TouchDetail) WasReleased() bool {
	return d.State&(touchMaskTouch|touchMaskChange) == touchMaskChange
}
func (d *TouchDetail) WasHold() bool    { return d.State == TouchHoldBegin }
func (d *TouchDetail) WasFlicked() bool { return d.State == TouchFlickEnd }
func (d *TouchDetail) WasDragged() bool { return d.State == TouchDragEnd }

const (
	touchMaxPoints = 2 // FT6336U reports up to two points

	// ref: M5Unified src/utility/Touch_Class.hpp (TOUCH_MIN_UPDATE_MSEC = 4, _flickThresh = 8, _msecHold = 500)
	touchMinUpdateMsec = 4
)

type touchPoint struct {
	x, y int16
	id   uint8
}

// TouchPanel is the FT6336U touch panel with the M5Unified touch state
// machine on top.
type TouchPanel struct {
	FlickThresh int32  // pixels, default 8
	HoldMsec    uint32 // default 500

	inited      bool
	flgReleased bool
	lastMsec    uint32
	count       int
	raw         [touchMaxPoints]touchPoint
	detail      [touchMaxPoints]TouchDetail
	buf         [2][1 + touchMaxPoints*6]byte
	w           [2]byte
}

// Touch is the touch panel, ready after InitTouch.
var Touch = TouchPanel{FlickThresh: 8, HoldMsec: 500}

var errTouchInit = errors.New("cores3: FT6336U init failed")

// InitTouch prepares the touch panel. Init must have been called before (it
// releases TOUCH_RST through AW9523B P0_0).
// ref: M5GFX src/lgfx/v1/touch/Touch_FT5x06.inl (Touch_FT5x06::init / _check_init)
// ref: M5GFX src/M5GFX.cpp (Touch_M5StackCoreS3: pin_int = GPIO_NUM_21, pin_rst = -1)
func InitTouch() error {
	// INT is the open-drain AW9523B INTN output.
	I2C_INT.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	t := &Touch
	t.inited = false
	t.flgReleased = false
	return t.checkInit()
}

func (t *TouchPanel) checkInit() error {
	if t.inited {
		return nil
	}
	var tmp [6]byte
	if err := t.writeReg(0x00, 0x00); err != nil {
		return err
	}
	t.w[0] = ft5x06RegCipher
	if err := I2C.Tx(AddrFT6336U, t.w[:1], tmp[:]); err != nil {
		return err
	}
	if err := t.writeReg(ft5x06RegIntMode, 0x00); err != nil { // INT polling mode
		return err
	}
	if tmp[5] == 0 { // VENDID (0xA8) must not be 0
		return errTouchInit
	}
	if Verbose {
		print("cores3: FT6336U CIPHER=0x")
		printHex8(tmp[0])
		print(" FIRMID=0x")
		printHex8(tmp[3])
		print(" VENDID=0x")
		printHex8(tmp[5])
		println()
	}
	t.inited = true
	return nil
}

func (t *TouchPanel) writeReg(reg, v byte) error {
	t.w[0], t.w[1] = reg, v
	return I2C.Tx(AddrFT6336U, t.w[:2], nil)
}

// readData reads TD_STATUS and the point data. Returns the number of valid
// bytes (1 when no point is touched), or 0 on error.
// ref: M5GFX Touch_FT5x06::_read_data
func (t *TouchPanel) readData(buf []byte) int {
	t.w[0] = ft5x06RegTDStatus
	if I2C.Tx(AddrFT6336U, t.w[:1], buf) != nil {
		return 0
	}
	points := int(buf[0] & 0x0F)
	if points > touchMaxPoints {
		points = touchMaxPoints
	}
	if points == 0 {
		return 1
	}
	return points*6 - 1
}

// getTouchRaw reads the raw points (panel coordinates).
// ref: M5GFX src/M5GFX.cpp (Touch_M5StackCoreS3::getTouchRaw) and
//
//	src/lgfx/v1/touch/Touch_FT5x06.inl (Touch_FT5x06::getTouchRaw)
func (t *TouchPanel) getTouchRaw() int {
	if t.checkInit() != nil {
		return 0
	}
	// CoreS3: TOUCH_INT -> AW9523B P1_2 -> INTN -> GPIO21. Read only while it is low.
	if I2C_INT.Get() {
		return 0
	}
	res := t.getTouchRawFT5x06()
	if res == 0 {
		// Reading input port 0 / 1 clears the AW9523B interrupt.
		IOExp.Input(aw9523b.Port0)
		IOExp.Input(aw9523b.Port1)
	}
	return res
}

func (t *TouchPanel) getTouchRawFT5x06() int {
	// The FT5x06 code checks the same INT pin again and switches the panel to
	// polling mode on every level change.
	if t.flgReleased != I2C_INT.Get() {
		t.flgReleased = !t.flgReleased
		t.writeReg(ft5x06RegIntMode, 0x00)
	}
	if t.flgReleased {
		return 0
	}
	var readlen [2]int
	if readlen[0] = t.readData(t.buf[0][:]); readlen[0] <= 1 {
		return 0
	}
	// The values can change while reading: read again until two reads match.
	for retry := 5; ; {
		readlen[retry&1] = t.readData(t.buf[retry&1][:])
		n := readlen[0]
		if readlen[1] < n {
			n = readlen[1]
		}
		retry--
		if (n != 0 && equal(t.buf[0][:readlen[0]], t.buf[1][:readlen[0]])) || retry == 0 {
			break
		}
	}
	count := int(t.buf[0][0] & 0x0F)
	if count > touchMaxPoints {
		count = touchMaxPoints
	}
	for i := 0; i < count; i++ {
		d := t.buf[0][i*6:]
		t.raw[i] = touchPoint{
			x:  int16(d[1]&0x0F)<<8 | int16(d[2]),
			y:  int16(d[3]&0x0F)<<8 | int16(d[4]),
			id: d[3] >> 4,
		}
	}
	return count
}

func equal(a, b []byte) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// convertRawXY maps panel coordinates to the current display rotation.
// Touch_M5StackCoreS3 has x 0..319 / y 0..239 (identity calibration) and
// offset_rotation 0, so only the panel rotation applies.
// ref: M5GFX src/lgfx/v1/panel/Panel_Device.inl (Panel_Device::convertRawXY)
func convertRawXY(p *touchPoint) {
	r := Display.Rotation()
	if r == 0 {
		return
	}
	w, h := Display.Size()
	tx, ty := p.x, p.y
	if r&1 != 0 {
		tx, ty = ty, tx
	}
	if r&2 != 0 {
		tx = (w - 1) - tx
	}
	if (1<<r)&0b10010110 != 0 { // r = 1, 2, 4, 7
		ty = (h - 1) - ty
	}
	p.x, p.y = tx, ty
}

// Update reads the panel and advances the state machine. Call it every loop
// with a millisecond clock.
// ref: M5Unified src/utility/Touch_Class.inl (Touch_Class::update)
func (t *TouchPanel) Update(msec uint32) {
	if msec-t.lastMsec <= touchMinUpdateMsec { // avoid high frequency updates
		if t.count == 0 {
			return
		}
		count := 0
		for i := range t.detail {
			if t.updateDetailTime(&t.detail[i], msec) {
				count++
			}
		}
		t.count = count
		return
	}
	t.lastMsec = msec
	count := t.getTouchRaw()
	if count == 0 && t.count == 0 {
		return
	}
	updated := uint32(0)
	for i := 0; i < count; i++ {
		p := t.raw[i]
		convertRawXY(&p)
		if p.id < touchMaxPoints {
			updated |= 1 << p.id
			t.updateDetail(&t.detail[p.id], msec, true, &p)
		}
	}
	for i := range t.detail {
		if updated&(1<<i) == 0 && t.updateDetail(&t.detail[i], msec, false, nil) && count < touchMaxPoints {
			count++
		}
	}
	t.count = count
}

// Count returns the number of active touch details.
func (t *TouchPanel) Count() int { return t.count }

// Detail returns the detail of the first raw point, like M5.Touch.getDetail().
func (t *TouchPanel) Detail() *TouchDetail {
	id := t.raw[0].id
	if id >= touchMaxPoints {
		id = 0
	}
	return &t.detail[id]
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// ref: M5Unified Touch_Class::update_detail(det, msec, pressed, tp)
func (t *TouchPanel) updateDetail(det *TouchDetail, msec uint32, pressed bool, tp *touchPoint) bool {
	tm := det.State
	if tm == TouchNone && !pressed {
		return false
	}
	tm &^= touchMaskChange
	if pressed {
		det.PrevX, det.PrevY = det.X, det.Y
		det.ID = tp.id
		if tm&touchMaskMoving == 0 { // not flicked yet
			if tm&touchMaskTouch != 0 { // not immediately after the touch
				if abs32(int32(det.BaseX)-int32(tp.x)) > t.FlickThresh ||
					abs32(int32(det.BaseY)-int32(tp.y)) > t.FlickThresh {
					det.PrevX, det.PrevY = det.BaseX, det.BaseY
					tm |= TouchFlickBegin
				} else if tm == TouchTouch && msec-det.BaseMsec > t.HoldMsec {
					tm = TouchHoldBegin
				}
			} else {
				det.X, det.Y = tp.x, tp.y
				tm = TouchTouchBegin
				if msec-det.BaseMsec > t.HoldMsec ||
					abs32(int32(det.BaseX)-int32(tp.x)) > (t.FlickThresh+1)<<2 ||
					abs32(int32(det.BaseY)-int32(tp.y)) > (t.FlickThresh+1)<<2 {
					det.ClickCount = 0
				}
				det.BaseMsec = msec
				det.BaseX, det.BaseY = tp.x, tp.y
				det.PrevX, det.PrevY = det.BaseX, det.BaseY
			}
		}
		if tm&touchMaskMoving != 0 {
			det.X, det.Y = tp.x, tp.y
		}
	} else {
		if tm&touchMaskTouch != 0 {
			tm = (tm | touchMaskChange) &^ touchMaskTouch
		} else {
			tm = TouchNone
		}
		if tm == TouchTouchEnd {
			det.BaseMsec = msec // for multi-tap detection
			det.ClickCount++
		}
	}
	det.State = tm
	return true
}

// ref: M5Unified Touch_Class::update_detail(det, msec)
func (t *TouchPanel) updateDetailTime(det *TouchDetail, msec uint32) bool {
	tm := det.State
	if tm == TouchNone {
		return false
	}
	tm &^= touchMaskChange
	if tm&TouchTouch != 0 {
		det.PrevX, det.PrevY = det.X, det.Y
		if tm == TouchTouch && msec-det.BaseMsec > t.HoldMsec {
			tm = TouchHoldBegin
		}
	} else {
		tm = TouchNone
	}
	det.State = tm
	return true
}
