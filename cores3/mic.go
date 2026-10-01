package cores3

import (
	"esp32-s3-lite/drivers/es7210"
	"esp32-s3-lite/internal/i2s"
)

// MicCodec is the ES7210, ready after StartMic.
var MicCodec es7210.Device

// MicNoiseFilterLevel is M5Unified's mic_config_t::noise_filter_level
// (0 = off, 1..255 = stronger low-pass).
var MicNoiseFilterLevel uint8

// micMagnification is the CoreS3 mic_cfg.magnification; with over_sampling 1
// the gain is magnification / (over_sampling << 1) = 1.
// ref: M5Unified src/M5Unified.inl (CoreS3 mic_cfg: magnification 2, over_sampling 1, input_stereo)
const micMagnification = 2

var (
	micOffset int32
	micPrev   [2]int32
	micRaw    [128]int16 // L, R pairs
)

// StartMic starts I2S1 RX (BCK=G34, WS=G33, DIN=G14, MCLK=G0, 240 MHz PLL)
// at rate and enables the ES7210. The speaker must be stopped (shared I2S1).
// ref: M5Unified src/M5Unified.inl (CoreS3 mic_cfg: pin_mck 0, pin_bck 34, pin_ws 33, pin_data_in 14, I2S_NUM_1)
// ref: M5Unified src/utility/Mic_Class.inl (mic_task: rx_clk_sel = 1, div_m = 8)
func StartMic(rate uint32) error {
	i2s.I2S1RX.Configure(i2s.Config{
		SampleRate: rate,
		BCK:        I2S_BCK,
		WS:         I2S_WS,
		MCLK:       I2S_MCLK,
		DataIn:     I2S_DATA_IN,
		Source:     i2s.SrcPLL240M,
	})
	i2s.I2S1RX.Start() // MCLK running before the codec is configured
	MicCodec = es7210.New(I2C)
	if err := MicCodec.Enable(); err != nil {
		return err
	}
	micOffset = 0
	micPrev = [2]int32{}
	// the first buffers after start-up are discarded, as M5Unified does (two reads)
	i2s.I2S1RX.Read(micRaw[:])
	i2s.I2S1RX.Read(micRaw[:])
	return nil
}

// StopMic stops I2S RX and puts the ES7210 in reset.
func StopMic() {
	i2s.I2S1RX.Stop()
	MicCodec.Disable()
}

// ReadMic records len(dst) mono samples (blocking), processed like M5Unified:
// automatic zero level, optional noise filter, gain, L/R average.
// ref: M5Unified src/utility/Mic_Class.inl (mic_task)
func ReadMic(dst []int16) {
	const fGain = micMagnification / 2 // magnification / (over_sampling << 1)
	for len(dst) > 0 {
		n := len(dst) * 2
		if n > len(micRaw) {
			n = len(micRaw)
		}
		i2s.I2S1RX.Read(micRaw[:n])
		nf := int32(MicNoiseFilterLevel)
		for i := 0; i < n; i += 2 {
			sv0, sv1 := int32(micRaw[i]), int32(micRaw[i+1])
			valueTmp := (sv0 + sv1) << 3
			micOffset -= (valueTmp + micOffset + 16) >> 5
			off := (micOffset + 8) >> 4
			s := [2]int32{sv0 + off, sv1 + off}
			for c := 0; c < 2; c++ {
				if nf != 0 {
					v := (s[c]*(256-nf) + micPrev[c]*nf + 128) >> 8
					micPrev[c] = v
					s[c] = v * fGain
				} else {
					s[c] *= fGain
				}
			}
			v := (s[0] + s[1] + 1) >> 1
			if v > 32767 {
				v = 32767
			} else if v < -32768 {
				v = -32768
			}
			dst[0] = int16(v)
			dst = dst[1:]
		}
	}
}
