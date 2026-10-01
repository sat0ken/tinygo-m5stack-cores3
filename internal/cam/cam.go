//go:build esp32s3

// Package cam captures single frames from a DVP (8 bit parallel) camera with
// the ESP32-S3 LCD_CAM peripheral and GDMA IN channel 1, written against the
// registers (TinyGo has no camera support).
//
// The frame boundary is taken from the LCD_CAM VSYNC interrupt flag (polled,
// no interrupt handler): wait for VSYNC, then start the DMA, then wait until
// the last descriptor of the frame buffer is filled.
//
// ref: espressif/esp32-camera target/esp32s3/ll_cam.c (ll_cam_config, ll_cam_set_pin,
// ll_cam_dma_reset, ll_cam_start, ll_cam_vsync_intr_enable), driver/cam_hal.c
// (vsync_invert = true, swap_data = 0, a frame is started on the VSYNC event)
// ref: ESP-IDF gdma_channel.h (SOC_GDMA_TRIG_PERIPH_CAM0 = 5), gpio_sig_map.h
package cam

import (
	"device/esp"
	"errors"
	"machine"
	"runtime/volatile"
	"time"
	"unsafe"
)

// GPIO matrix input signals. ref: gpio_sig_map.h
const (
	sigCamDataIn0  = 133
	sigCamPCLK     = 149
	sigCamHEnable  = 150
	sigCamVSync    = 152
	gdmaPeriphCam0 = 5
)

// nodeSize is the bytes per DMA descriptor: a multiple of a QVGA RGB565 line
// (640 bytes) below LCD_CAM_DMA_NODE_BUFFER_MAX_SIZE (4092).
const nodeSize = 640 * 6

// Pins is the DVP wiring.
type Pins struct {
	Data              [8]machine.Pin // D0..D7
	VSync, HRef, PCLK machine.Pin
}

var (
	ErrBufferSize = errors.New("cam: buffer size must be a multiple of 3840 bytes")
	ErrTimeout    = errors.New("cam: timeout waiting for VSYNC")
	ErrTimeoutDMA = errors.New("cam: timeout waiting for the frame data")
)

type dmaDesc struct {
	dw0, buffer, next uint32
}

// Camera holds the descriptor chain (must stay at a fixed address).
type Camera struct {
	desc []dmaDesc
}

// Cam is the only instance.
var Cam Camera

// Debug prints the DMA state when a capture times out.
var Debug = true

func inFunc(signal uint32) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.GPIO.FUNC0_IN_SEL_CFG), uintptr(signal)*4))
}

// routeIn connects pin p to an input signal. ref: ll_cam_set_pin (gpio_matrix_in)
func routeIn(p machine.Pin, signal uint32, invert bool) {
	p.Configure(machine.PinConfig{Mode: machine.PinInput})
	v := esp.GPIO_FUNC_IN_SEL_CFG_SEL | uint32(p)<<esp.GPIO_FUNC_IN_SEL_CFG_IN_SEL_Pos
	if invert {
		v |= 1 << 6 // GPIO_FUNCx_IN_INV_SEL
	}
	inFunc(signal).Set(v)
}

// Configure sets up LCD_CAM in camera mode (8 bit, PCLK from the sensor, no
// XCLK output) and GDMA IN channel 1.
// ref: ll_cam_config / ll_cam_set_pin / ll_cam_dma_reset
func (c *Camera) Configure(p Pins) {
	// periph_ll_enable_clk_clear_rst(PERIPH_LCD_CAM_MODULE)
	esp.SYSTEM.SetPERIP_CLK_EN1_LCD_CAM_CLK_EN(1)
	esp.SYSTEM.SetPERIP_RST_EN1_LCD_CAM_RST(1)
	esp.SYSTEM.SetPERIP_RST_EN1_LCD_CAM_RST(0)
	// GDMA: clock on and out of reset. The reset bit is set after boot; the
	// I2S driver happened to clear it, a camera-only program must do it.
	// ref: ESP-IDF gdma_ll.h (_gdma_ll_enable_bus_clock / _gdma_ll_reset_register)
	if esp.SYSTEM.GetPERIP_CLK_EN1_DMA_CLK_EN() == 0 {
		esp.SYSTEM.SetPERIP_CLK_EN1_DMA_CLK_EN(1)
		esp.SYSTEM.SetPERIP_RST_EN1_DMA_RST(1)
	}
	esp.SYSTEM.SetPERIP_RST_EN1_DMA_RST(0)
	esp.DMA.SetMISC_CONF_CLK_EN(1)

	routeIn(p.PCLK, sigCamPCLK, false)
	routeIn(p.VSync, sigCamVSync, true) // cam_hal.c: vsync_invert = true
	routeIn(p.HRef, sigCamHEnable, false)
	for i, d := range p.Data {
		routeIn(d, sigCamDataIn0+uint32(i), false)
	}

	l := esp.LCD_CAM
	l.CAM_CTRL.Set(0)
	l.SetCAM_CTRL_CAM_CLKM_DIV_B(0)
	l.SetCAM_CTRL_CAM_CLKM_DIV_A(0)
	l.SetCAM_CTRL_CAM_CLKM_DIV_NUM(160_000_000 / 20_000_000) // xclk_freq_hz = 20 MHz in the tutorial (not output)
	l.SetCAM_CTRL_CAM_CLK_SEL(3)                             // "no clock": data is sampled with PCLK
	l.SetCAM_CTRL_CAM_STOP_EN(0)
	l.SetCAM_CTRL_CAM_VSYNC_FILTER_THRES(4)
	l.SetCAM_CTRL_CAM_BYTE_ORDER(0) // cam_hal.c: swap_data = 0
	l.SetCAM_CTRL_CAM_BIT_ORDER(0)
	l.SetCAM_CTRL_CAM_LINE_INT_EN(0)
	l.SetCAM_CTRL_CAM_VS_EOF_EN(0)
	l.CAM_CTRL1.Set(0)
	l.SetCAM_CTRL1_CAM_REC_DATA_BYTELEN(nodeSize - 1) // in_suc_eof after every descriptor
	l.SetCAM_CTRL1_CAM_VSYNC_FILTER_EN(1)
	l.CAM_RGB_YUV.Set(0)
	l.SetCAM_CTRL_CAM_UPDATE(1)
	// ll_cam_config ends with cam_start = 1: the VSYNC event is only detected
	// while the camera block runs (measured: LC_DMA_INT_RAW stays 0 otherwise).
	l.SetCAM_CTRL1_CAM_START(1)

	// ll_cam_dma_reset (internal SRAM: descriptor and data burst on)
	d := esp.DMA
	d.IN_INT_CLR_CH1.Set(^uint32(0))
	d.IN_INT_ENA_CH1.Set(0)
	d.IN_CONF0_CH1.Set(0)
	d.SetIN_CONF0_CH1_IN_RST(1)
	d.SetIN_CONF0_CH1_IN_RST(0)
	d.SetIN_CONF0_CH1_INDSCR_BURST_EN(1)
	d.SetIN_CONF0_CH1_IN_DATA_BURST_EN(1)
	d.SetIN_CONF1_CH1_IN_CHECK_OWNER(0)
	d.SetIN_PERI_SEL_CH1_PERI_IN_SEL(gdmaPeriphCam0)
}

// Capture fills buf (len = width * height * 2, a multiple of 3840 bytes)
// with the next complete frame. buf must be in internal RAM.
func (c *Camera) Capture(buf []byte, timeout time.Duration) error {
	if len(buf) == 0 || len(buf)%nodeSize != 0 {
		return ErrBufferSize
	}
	n := len(buf) / nodeSize
	if len(c.desc) != n {
		c.desc = make([]dmaDesc, n)
	}
	for i := range c.desc {
		c.desc[i].dw0 = uint32(nodeSize) | 1<<31 // size, owner = DMA
		c.desc[i].buffer = uint32(uintptr(unsafe.Pointer(&buf[i*nodeSize])))
		if i+1 < n {
			c.desc[i].next = uint32(uintptr(unsafe.Pointer(&c.desc[i+1])))
		} else {
			c.desc[i].next = 0
		}
	}
	last := uint32(uintptr(unsafe.Pointer(&c.desc[n-1])))

	l := esp.LCD_CAM
	d := esp.DMA
	deadline := time.Now().Add(timeout)

	// Wait for the start of a frame (VSYNC event) with the camera block
	// running. ref: cam_hal.c (CAM_VSYNC_EVENT -> cam_start_frame -> ll_cam_start)
	l.SetCAM_CTRL1_CAM_START(1)
	l.SetLC_DMA_INT_CLR_CAM_VSYNC_INT_CLR(1)
	for l.GetLC_DMA_INT_RAW_CAM_VSYNC_INT_RAW() == 0 {
		if time.Now().After(deadline) {
			return ErrTimeout
		}
	}
	l.SetLC_DMA_INT_CLR_CAM_VSYNC_INT_CLR(1)

	// ref: ll_cam_start
	l.SetCAM_CTRL1_CAM_START(0)
	l.SetCAM_CTRL1_CAM_RESET(1)
	l.SetCAM_CTRL1_CAM_RESET(0)
	l.SetCAM_CTRL1_CAM_AFIFO_RESET(1)
	l.SetCAM_CTRL1_CAM_AFIFO_RESET(0)
	d.SetIN_CONF0_CH1_IN_RST(1)
	d.SetIN_CONF0_CH1_IN_RST(0)
	d.IN_SUC_EOF_DES_ADDR_CH1.Get()
	d.IN_INT_CLR_CH1.Set(^uint32(0))
	d.SetIN_LINK_CH1_INLINK_ADDR(uint32(uintptr(unsafe.Pointer(&c.desc[0]))) & 0xFFFFF)
	d.SetIN_LINK_CH1_INLINK_START(1)
	l.SetCAM_CTRL_CAM_UPDATE(1)
	l.SetCAM_CTRL1_CAM_START(1)

	for d.IN_SUC_EOF_DES_ADDR_CH1.Get() != last {
		if time.Now().After(deadline) {
			if Debug {
				base := uint32(uintptr(unsafe.Pointer(&c.desc[0])))
				eof := d.IN_SUC_EOF_DES_ADDR_CH1.Get()
				println("cam: dma timeout: suc_eof desc", int(eof-base)/12, "of", n, "eof addr", eof, "base", base,
					"IN_STATE", d.IN_STATE_CH1.Get(), "IN_DSCR", d.IN_DSCR_CH1.Get(), "INFIFO", d.INFIFO_STATUS_CH1.Get(),
					"IN_INT_RAW", d.IN_INT_RAW_CH1.Get(), "desc0.dw0", c.desc[0].dw0, "LC_INT_RAW", l.LC_DMA_INT_RAW.Get())
			}
			return ErrTimeoutDMA
		}
	}
	// stop the DMA so that the buffer is not overwritten; the camera block
	// keeps running for the next VSYNC detection
	d.SetIN_LINK_CH1_INLINK_STOP(1)
	return nil
}
