package cores3

import (
	"machine"
	"time"

	"esp32-s3-lite/drivers/gc0308"
	"esp32-s3-lite/internal/cam"
)

// Camera (GC0308, DVP) pins. XCLK is not used (pin_xclk = -1).
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/camera (camera_config_t)
var cameraPins = cam.Pins{
	Data:  [8]machine.Pin{machine.GPIO39, machine.GPIO40, machine.GPIO41, machine.GPIO42, machine.GPIO15, machine.GPIO16, machine.GPIO48, machine.GPIO47},
	VSync: machine.GPIO46,
	HRef:  machine.GPIO38,
	PCLK:  machine.GPIO45,
}

// Photo size: QVGA RGB565.
const (
	PhotoWidth  = 320
	PhotoHeight = 240
)

// CameraSensor is the GC0308, ready after InitCamera.
var CameraSensor gc0308.Device

// InitCamera configures the GC0308 (RGB565, 320x240) and the capture
// hardware. Init must have been called (camera power ALDO3, CAM_RST high).
func InitCamera() error {
	CameraSensor = gc0308.New(I2C)
	if err := CameraSensor.Configure(); err != nil {
		return err
	}
	cam.Cam.Configure(cameraPins)
	return nil
}

// CapturePhoto fills buf (PhotoWidth * PhotoHeight * 2 bytes) with one frame
// in the byte order sent by the sensor.
func CapturePhoto(buf []byte) error {
	return cam.Cam.Capture(buf, time.Second)
}
