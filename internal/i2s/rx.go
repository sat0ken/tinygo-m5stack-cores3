//go:build esp32s3

package i2s

import (
	"device/esp"
	"machine"
	"time"
	"unsafe"
)

const (
	rxNumDesc  = 8
	rxBufBytes = 1024 // per descriptor (256 stereo 16 bit frames)
)

// RX is the I2S1 receiver (master: BCK, WS and MCLK are outputs).
type RX struct {
	desc    [rxNumDesc]dmaDesc
	buf     [rxNumDesc][rxBufBytes]byte
	started bool
	ri      int // next descriptor to read
	rpos    int // bytes consumed in descriptor ri
	lastEOF int
	ready   int // filled descriptors not yet read
	// Overruns counts descriptors the DMA overwrote before they were read.
	Overruns int
}

// I2S1RX is the only instance.
var I2S1RX RX

// Configure sets up I2S1 RX (Philips, 16 bit x 2 slots, master) and GDMA
// IN channel 0. MCLK = 256 fs is output on cfg.MCLK.
// ref: i2s_hal_set_rx_clock, i2s_hal_std_set_rx_slot (SOC_I2S_HW_VERSION_2),
// i2s_common.c (i2s_ll_rx_set_eof_num(dev, bufsize))
func (t *RX) Configure(cfg Config) {
	enableBus()

	routeOut(cfg.BCK, sigI2S1IBCK)
	routeOut(cfg.WS, sigI2S1IWS)
	routeOut(cfg.MCLK, sigI2S1MCLK)
	if cfg.DataIn != machine.NoPin {
		cfg.DataIn.Configure(machine.PinConfig{Mode: machine.PinInput})
		inFunc(sigI2S1ISD).Set(esp.GPIO_FUNC_IN_SEL_CFG_SEL | uint32(cfg.DataIn)<<esp.GPIO_FUNC_IN_SEL_CFG_IN_SEL_Pos)
	}

	r := esp.I2S1
	r.SetTX_CLKM_CONF_CLK_EN(1) // module clock gate (in the TX register)

	src := cfg.Source
	if src == 0 {
		src = SrcPLL160M
	}
	r.SetRX_CLKM_CONF_RX_CLK_ACTIVE(1)
	r.SetRX_CLKM_CONF_MCLK_SEL(1) // i2s_ll_mclk_bind_to_rx_clk
	r.SetRX_CLKM_CONF_RX_CLK_SEL(uint32(src))
	n, x, y, z, yn1 := mclkDiv(srcFreq(src), cfg.SampleRate*256)
	r.SetRX_CLKM_CONF_RX_CLKM_DIV_NUM(2)
	r.SetRX_CLKM_DIV_CONF_RX_CLKM_DIV_YN1(0)
	r.SetRX_CLKM_DIV_CONF_RX_CLKM_DIV_Y(1)
	r.SetRX_CLKM_DIV_CONF_RX_CLKM_DIV_Z(0)
	r.SetRX_CLKM_DIV_CONF_RX_CLKM_DIV_X(0)
	r.SetRX_CLKM_DIV_CONF_RX_CLKM_DIV_YN1(yn1)
	r.SetRX_CLKM_DIV_CONF_RX_CLKM_DIV_Z(z)
	r.SetRX_CLKM_DIV_CONF_RX_CLKM_DIV_Y(y)
	r.SetRX_CLKM_DIV_CONF_RX_CLKM_DIV_X(x)
	r.SetRX_CLKM_CONF_RX_CLKM_DIV_NUM(n)
	// BCK = MCLK / 8 = 32 fs. M5Unified: "the CoreS3 mic needs div_m >= 8".
	r.SetRX_CONF1_RX_BCK_DIV_NUM(8 - 1)

	r.SetRX_CONF_RX_RESET(1)
	r.SetRX_CONF_RX_RESET(0)
	r.SetRX_CONF_RX_SLAVE_MOD(0)
	r.SetRX_CONF1_RX_BITS_MOD(16 - 1)
	r.SetRX_CONF1_RX_TDM_CHAN_BITS(16 - 1)
	r.SetRX_CONF_RX_MONO(0) // stereo
	r.SetRX_CONF_RX_MONO_FST_VLD(0)
	r.SetRX_CONF1_RX_MSB_SHIFT(1)
	r.SetRX_CONF1_RX_TDM_WS_WIDTH(16 - 1)
	r.SetRX_TDM_CTRL_RX_TDM_TOT_CHAN_NUM(1)
	r.RX_TDM_CTRL.Set(r.RX_TDM_CTRL.Get()&0xFFFF0000 | 0x03) // both slots
	r.SetRX_CONF1_RX_HALF_SAMPLE_BITS(16 - 1)
	r.SetRX_CONF_RX_WS_IDLE_POL(0)
	r.SetRX_CONF_RX_BIT_ORDER(0)
	r.SetRX_CONF_RX_LEFT_ALIGN(1)
	r.SetRX_CONF_RX_BIG_ENDIAN(0)
	r.SetRX_CONF_RX_PDM_EN(0)
	r.SetRX_CONF_RX_TDM_EN(1)
	r.SetRXEOF_NUM_RX_EOF_NUM(rxBufBytes)

	for i := range t.desc {
		next := (i + 1) % rxNumDesc
		t.desc[i].dw0 = uint32(rxBufBytes) | 1<<31 // size, owner=DMA
		t.desc[i].buffer = uint32(uintptr(unsafe.Pointer(&t.buf[i][0])))
		t.desc[i].next = uint32(uintptr(unsafe.Pointer(&t.desc[next])))
	}
	t.started = false
}

// Start starts receiving into the descriptor ring.
// ref: ESP-IDF i2s_common.c (rx start), M5Unified Mic_Class mic_task ("If RX is
// not reset here, BCK polarity may be inverted")
func (t *RX) Start() {
	r := esp.I2S1
	r.SetRX_CONF_RX_RESET(1)
	r.SetRX_CONF_RX_FIFO_RESET(1)
	r.SetRX_CONF_RX_RESET(0)
	r.SetRX_CONF_RX_FIFO_RESET(0)

	d := esp.DMA
	d.SetIN_CONF0_CH0_IN_RST(1)
	d.SetIN_CONF0_CH0_IN_RST(0)
	d.SetIN_CONF1_CH0_IN_CHECK_OWNER(0)
	d.SetIN_CONF0_CH0_MEM_TRANS_EN(0)
	d.SetIN_PERI_SEL_CH0_PERI_IN_SEL(gdmaPeriphI2S1)
	d.SetIN_LINK_CH0_INLINK_ADDR(uint32(uintptr(unsafe.Pointer(&t.desc[0]))) & 0xFFFFF)
	d.SetIN_LINK_CH0_INLINK_START(1)
	t.ri, t.rpos, t.ready, t.Overruns = 0, 0, 0, 0
	t.lastEOF = rxNumDesc - 1

	r.SetRX_CONF_RX_UPDATE(1)
	for r.GetRX_CONF_RX_UPDATE() != 0 {
	}
	r.SetRX_CONF_RX_START(1)
	t.started = true
}

// Stop stops receiving.
func (t *RX) Stop() {
	esp.I2S1.SetRX_CONF_RX_START(0)
	esp.DMA.SetIN_LINK_CH0_INLINK_STOP(1)
	t.started = false
}

func (t *RX) poll() {
	addr := esp.DMA.IN_SUC_EOF_DES_ADDR_CH0.Get()
	base := uint32(uintptr(unsafe.Pointer(&t.desc[0])))
	if addr < base || addr >= base+uint32(rxNumDesc)*12 {
		return
	}
	idx := int(addr-base) / 12
	for t.lastEOF != idx {
		t.lastEOF = (t.lastEOF + 1) % rxNumDesc
		t.ready++
	}
	if t.ready > rxNumDesc-1 {
		// the DMA has overwritten unread buffers: skip to the oldest valid one
		t.Overruns += t.ready - (rxNumDesc - 1)
		t.ready = rxNumDesc - 1
		t.ri = (t.lastEOF + 2) % rxNumDesc
		t.rpos = 0
	}
}

// Read fills dst with interleaved L, R 16 bit samples, blocking until enough
// data has been received.
func (t *RX) Read(dst []int16) {
	for len(dst) > 0 {
		t.poll()
		if t.ready == 0 {
			time.Sleep(time.Millisecond)
			continue
		}
		b := t.buf[t.ri][:]
		for t.rpos < rxBufBytes && len(dst) > 0 {
			dst[0] = int16(uint16(b[t.rpos]) | uint16(b[t.rpos+1])<<8)
			dst = dst[1:]
			t.rpos += 2
		}
		if t.rpos == rxBufBytes {
			t.rpos = 0
			t.ri = (t.ri + 1) % rxNumDesc
			t.ready--
		}
	}
}
