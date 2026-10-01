//go:build esp32s3

// Package i2s is a minimal ESP32-S3 I2S1 driver (standard / Philips format,
// master, 16 bit) with GDMA, written directly against the registers because
// TinyGo has no I2S support for the ESP32 family.
//
// The ESP32-S3 I2S has no CPU-accessible data FIFO (unlike the ESP32): data
// can only move through GDMA. TX uses a ring of DMA descriptors that the CPU
// refills by polling the "EOF descriptor address" register.
//
// ref: ESP-IDF components/esp_hal_i2s/esp32s3/include/hal/i2s_ll.h, components/esp_hal_i2s/i2s_hal.c
// ref: ESP-IDF components/esp_hal_dma/esp32s3/include/hal/gdma_ll.h, gdma_channel.h
// ref: ESP-IDF components/esp_hal_dma/include/hal/dma_types.h (dma_descriptor_t)
// ref: ESP-IDF components/soc/esp32s3/include/soc/gpio_sig_map.h
// ref: ESP32-S3 Technical Reference Manual, chapter "I2S Controller", "GDMA Controller"
package i2s

import (
	"device/esp"
	"machine"
	"runtime/volatile"
	"time"
	"unsafe"
)

// GPIO matrix signal indices of I2S1. ref: gpio_sig_map.h
const (
	sigI2S1MCLK    = 21 // I2S1_MCLK_OUT_IDX
	sigI2S1OBCK    = 28 // I2S1O_BCK_OUT_IDX
	sigI2S1OWS     = 29 // I2S1O_WS_OUT_IDX
	sigI2S1OSD     = 30 // I2S1O_SD_OUT_IDX
	sigI2S1ISD     = 30 // I2S1I_SD_IN_IDX
	sigI2S1IBCK    = 31 // I2S1I_BCK_OUT_IDX
	sigI2S1IWS     = 32 // I2S1I_WS_OUT_IDX
	gdmaPeriphI2S1 = 4  // SOC_GDMA_TRIG_PERIPH_I2S1
)

// Clock sources (tx_clk_sel / rx_clk_sel). ref: i2s_ll_tx_clk_set_src
const (
	// SrcPLL160M is the ESP-IDF default on the S3
	// (clk_tree_defs.h: I2S_CLK_SRC_DEFAULT = SOC_MOD_CLK_PLL_F160M).
	SrcPLL160M = 2
	// SrcPLL240M is what M5Unified selects for the microphone
	// (Mic_Class.inl: rx_clk_sel = 1 "PLL_240M_CLK").
	SrcPLL240M = 1
)

func srcFreq(src uint8) uint32 {
	if src == SrcPLL240M {
		return 240_000_000
	}
	return 160_000_000
}

// Config is the TX configuration.
type Config struct {
	SampleRate uint32
	BCK, WS    machine.Pin
	DataOut    machine.Pin
	MCLK       machine.Pin // machine.NoPin when not used
	// Mono sends each 16 bit sample to both slots (ESP-IDF I2S_SLOT_MODE_MONO
	// with I2S_STD_SLOT_BOTH). Otherwise samples are interleaved L, R.
	// (TX only; RX always delivers L, R pairs.)
	Mono bool
	// Source is SrcPLL160M (default when 0) or SrcPLL240M.
	Source uint8
	// DataIn is the RX data pin.
	DataIn machine.Pin
}

// enableBus turns on and resets I2S1 and GDMA.
// ref: i2s_ll_enable_bus_clock / i2s_ll_reset_register, _gdma_ll_enable_bus_clock / _gdma_ll_reset_register
func enableBus() {
	esp.SYSTEM.SetPERIP_CLK_EN0_I2S1_CLK_EN(1)
	esp.SYSTEM.SetPERIP_RST_EN0_I2S1_RST(1)
	esp.SYSTEM.SetPERIP_RST_EN0_I2S1_RST(0)
	esp.SYSTEM.SetPERIP_CLK_EN1_DMA_CLK_EN(1)
	esp.SYSTEM.SetPERIP_RST_EN1_DMA_RST(1)
	esp.SYSTEM.SetPERIP_RST_EN1_DMA_RST(0)
	esp.DMA.SetMISC_CONF_CLK_EN(1) // gdma_ll_force_enable_reg_clock
}

const (
	numDesc  = 8
	bufBytes = 1024 // per descriptor; multiple of 4, < 4096
)

// dmaDesc is dma_descriptor_t (12 bytes, 4-byte aligned).
type dmaDesc struct {
	dw0    uint32 // size[11:0], length[23:12], suc_eof[30], owner[31]
	buffer uint32
	next   uint32
}

// TX is the I2S1 transmitter.
type TX struct {
	desc     [numDesc]dmaDesc
	buf      [numDesc][bufBytes]byte
	started  bool
	wi       int // slot being filled
	wpos     int // bytes filled in slot wi
	inflight int // filled slots not yet played
	lastEOF  int // index of the last descriptor reported as done
}

// I2S1TX is the only instance (descriptors must stay at a fixed address).
var I2S1TX TX

func outFunc(p machine.Pin) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.GPIO.FUNC0_OUT_SEL_CFG), uintptr(p)*4))
}

func inFunc(signal uint32) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.GPIO.FUNC0_IN_SEL_CFG), uintptr(signal)*4))
}

func routeOut(p machine.Pin, signal uint32) {
	if p == machine.NoPin {
		return
	}
	p.Configure(machine.PinConfig{Mode: machine.PinOutput})
	outFunc(p).Set(signal)
}

// Configure sets up I2S1 TX and GDMA OUT channel 0. Call Start to begin
// clocking out (silence).
func (t *TX) Configure(cfg Config) {
	enableBus()

	routeOut(cfg.BCK, sigI2S1OBCK)
	routeOut(cfg.WS, sigI2S1OWS)
	routeOut(cfg.DataOut, sigI2S1OSD)
	routeOut(cfg.MCLK, sigI2S1MCLK)

	r := esp.I2S1
	r.SetTX_CLKM_CONF_CLK_EN(1) // i2s_ll_enable_core_clock

	// Clock. ref: i2s_hal_set_tx_clock: tx_clk_active = 1, mclk_sel = 0 (MCLK from TX),
	// tx_clk_sel, mclk division, bck division.
	// MCLK = 256 * fs (I2S_MCLK_MULTIPLE_256, as M5Unified), BCK = fs * 16 bit * 2 slots.
	mclk := cfg.SampleRate * 256
	bckDiv := uint32(256 / 32)
	r.SetTX_CLKM_CONF_TX_CLK_ACTIVE(1)
	r.SetRX_CLKM_CONF_MCLK_SEL(0)
	src := cfg.Source
	if src == 0 {
		src = SrcPLL160M
	}
	r.SetTX_CLKM_CONF_TX_CLK_SEL(uint32(src))
	n, x, y, z, yn1 := mclkDiv(srcFreq(src), mclk)
	// ref: i2s_ll_tx_set_raw_clk_div ("set a small division first" workaround)
	r.SetTX_CLKM_CONF_TX_CLKM_DIV_NUM(2)
	r.SetTX_CLKM_DIV_CONF_TX_CLKM_DIV_YN1(0)
	r.SetTX_CLKM_DIV_CONF_TX_CLKM_DIV_Y(1)
	r.SetTX_CLKM_DIV_CONF_TX_CLKM_DIV_Z(0)
	r.SetTX_CLKM_DIV_CONF_TX_CLKM_DIV_X(0)
	r.SetTX_CLKM_DIV_CONF_TX_CLKM_DIV_YN1(yn1)
	r.SetTX_CLKM_DIV_CONF_TX_CLKM_DIV_Z(z)
	r.SetTX_CLKM_DIV_CONF_TX_CLKM_DIV_Y(y)
	r.SetTX_CLKM_DIV_CONF_TX_CLKM_DIV_X(x)
	r.SetTX_CLKM_CONF_TX_CLKM_DIV_NUM(n)
	r.SetTX_CONF1_TX_BCK_DIV_NUM(bckDiv - 1)

	// Slot (standard / Philips, 16 bit). ref: i2s_hal_std_set_tx_slot (SOC_I2S_HW_VERSION_2)
	r.SetTX_CONF_TX_RESET(1)
	r.SetTX_CONF_TX_RESET(0)
	r.SetTX_CONF_TX_SLAVE_MOD(0)
	r.SetTX_CONF1_TX_BITS_MOD(16 - 1)
	r.SetTX_CONF1_TX_TDM_CHAN_BITS(16 - 1)
	r.SetTX_CONF1_TX_MSB_SHIFT(1)         // Philips: 1 BCK delay
	r.SetTX_CONF1_TX_TDM_WS_WIDTH(16 - 1) // ws_width = slot bits
	mono := uint32(0)
	chanMask := uint32(0x03)
	if cfg.Mono {
		mono = 1
		chanMask = 0x01 // is_copy_mono: select LEFT, the other slot copies it
	}
	r.SetTX_CONF_TX_MONO(mono)
	r.SetTX_CONF_TX_CHAN_EQUAL(mono)
	r.SetTX_TDM_CTRL_TX_TDM_TOT_CHAN_NUM(1) // 2 slots
	r.TX_TDM_CTRL.Set(r.TX_TDM_CTRL.Get()&0xFFFF0000 | chanMask)
	r.SetTX_TDM_CTRL_TX_TDM_SKIP_MSK_EN(0)
	r.SetTX_CONF1_TX_HALF_SAMPLE_BITS(16 - 1)
	r.SetTX_CONF_TX_WS_IDLE_POL(0)
	r.SetTX_CONF_TX_BIT_ORDER(0)
	r.SetTX_CONF_TX_LEFT_ALIGN(1)
	r.SetTX_CONF_TX_BIG_ENDIAN(0)
	// i2s_ll_tx_enable_std -> enable_tdm (I2S1 has no PCM2PDM converter)
	r.SetTX_CONF_TX_PDM_EN(0)
	r.SetTX_CONF_TX_TDM_EN(1)

	// Descriptor ring.
	for i := range t.desc {
		next := (i + 1) % numDesc
		t.desc[i].dw0 = uint32(bufBytes) | uint32(bufBytes)<<12 | 1<<30 | 1<<31 // size, length, suc_eof, owner=DMA
		t.desc[i].buffer = uint32(uintptr(unsafe.Pointer(&t.buf[i][0])))
		t.desc[i].next = uint32(uintptr(unsafe.Pointer(&t.desc[next])))
		clear(t.buf[i][:])
	}
	t.started = false
	t.wi, t.wpos, t.inflight = 0, 0, 0
	t.lastEOF = numDesc - 1
}

// mclkDiv returns the MCLK divider src / (N + b/a) in the register encoding
// (div_num, x, y, z, yn1).
// ref: i2s_ll_tx_set_mclk (yn1/x/y/z from numerator/denominator),
// hal_utils_calc_clk_div_frac_accurate (best fraction, denominator < 512)
func mclkDiv(src, mclk uint32) (n, x, y, z, yn1 uint32) {
	n = src / mclk
	rem := src % mclk
	num, den := uint32(0), uint32(1)
	if rem != 0 {
		bestErr := uint64(1 << 63)
		for d := uint32(2); d < 512; d++ {
			nn := (uint64(rem)*uint64(d) + uint64(mclk)/2) / uint64(mclk)
			if nn == 0 || nn >= uint64(d) {
				continue
			}
			a, b := uint64(rem)*uint64(d), nn*uint64(mclk)
			e := a - b
			if b > a {
				e = b - a
			}
			e = e * 1024 / uint64(d)
			if e < bestErr {
				bestErr, num, den = e, uint32(nn), d
				if e == 0 {
					break
				}
			}
		}
	}
	if num != 0 && den != 0 {
		if num*2 > den {
			yn1 = 1
			z = den - num
		} else {
			z = num
		}
		x = den/z - 1
		y = den % z
	}
	return
}

// Start resets the FIFO and DMA and starts clocking out the ring.
// ref: ESP-IDF i2s_common.c (i2s_tx_channel_start): reset tx, fifo, dma, gdma_start, i2s_ll_tx_start
func (t *TX) Start() {
	if t.started {
		return
	}
	r := esp.I2S1
	r.SetTX_CONF_TX_RESET(1)
	r.SetTX_CONF_TX_RESET(0)
	r.SetTX_CONF_TX_FIFO_RESET(1)
	r.SetTX_CONF_TX_FIFO_RESET(0)

	d := esp.DMA
	d.SetOUT_CONF0_CH0_OUT_RST(1)
	d.SetOUT_CONF0_CH0_OUT_RST(0)
	d.SetOUT_CONF1_CH0_OUT_CHECK_OWNER(0)
	d.SetOUT_CONF0_CH0_OUT_EOF_MODE(1) // EOF when the data has been popped from the FIFO
	d.SetOUT_PERI_SEL_CH0_PERI_OUT_SEL(gdmaPeriphI2S1)
	d.SetOUT_LINK_CH0_OUTLINK_ADDR(uint32(uintptr(unsafe.Pointer(&t.desc[t.wi]))) & 0xFFFFF)
	t.lastEOF = (t.wi + numDesc - 1) % numDesc
	d.SetOUT_LINK_CH0_OUTLINK_START(1)

	// i2s_ll_tx_start: update, then start
	r.SetTX_CONF_TX_UPDATE(1)
	for r.GetTX_CONF_TX_UPDATE() != 0 {
	}
	r.SetTX_CONF_TX_START(1)
	t.started = true
}

// poll accounts for descriptors the DMA has finished and clears them, so
// that an underrun plays silence instead of stale audio.
func (t *TX) poll() {
	addr := esp.DMA.OUT_EOF_DES_ADDR_CH0.Get()
	base := uint32(uintptr(unsafe.Pointer(&t.desc[0])))
	if addr < base || addr >= base+uint32(numDesc)*12 {
		return
	}
	idx := int(addr-base) / 12
	for t.lastEOF != idx {
		t.lastEOF = (t.lastEOF + 1) % numDesc
		if t.lastEOF != t.wi {
			clear(t.buf[t.lastEOF][:])
		}
		if t.inflight > 0 {
			t.inflight--
		}
	}
	if t.inflight == 0 && t.started {
		// underrun: continue writing just ahead of the DMA
		next := (idx + 2) % numDesc
		if t.wi != next && t.wpos == 0 {
			t.wi = next
		}
	}
}

// Write queues 16 bit little-endian samples (mono, or L/R interleaved),
// blocking while the ring is full. It starts the transmitter if needed.
func (t *TX) Write(samples []int16) {
	for len(samples) > 0 {
		t.poll()
		if t.inflight >= numDesc-2 {
			if !t.started {
				t.Start()
			}
			time.Sleep(time.Millisecond)
			continue
		}
		b := t.buf[t.wi][:]
		for t.wpos < bufBytes && len(samples) > 0 {
			v := uint16(samples[0])
			b[t.wpos], b[t.wpos+1] = byte(v), byte(v>>8)
			t.wpos += 2
			samples = samples[1:]
		}
		if t.wpos == bufBytes {
			t.wpos = 0
			t.wi = (t.wi + 1) % numDesc
			t.inflight++
			if !t.started && t.inflight >= 2 {
				t.Start()
			}
		}
	}
}

// Flush pads the current buffer with silence and waits until everything
// queued has been played. The transmitter keeps running (silence).
func (t *TX) Flush() {
	if t.wpos > 0 {
		clear(t.buf[t.wi][t.wpos:])
		t.wpos = 0
		t.wi = (t.wi + 1) % numDesc
		t.inflight++
	}
	if !t.started {
		t.Start()
	}
	for t.inflight > 0 {
		time.Sleep(time.Millisecond)
		t.poll()
	}
}

// Stop stops the transmitter and the DMA.
func (t *TX) Stop() {
	esp.I2S1.SetTX_CONF_TX_START(0)
	esp.DMA.SetOUT_LINK_CH0_OUTLINK_STOP(1)
	t.started = false
}
