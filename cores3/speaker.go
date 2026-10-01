package cores3

import (
	"machine"

	"esp32-s3-lite/drivers/aw88298"
	"esp32-s3-lite/drivers/aw9523b"
	"esp32-s3-lite/internal/i2s"
)

// SpeakerSampleRate is the default I2S rate for the speaker.
// ref: M5Unified src/utility/Speaker_Class.hpp (sample_rate = 48000)
const SpeakerSampleRate = 48000

// Amp is the speaker amplifier, ready after StartSpeaker.
var Amp aw88298.Device

var speakerRate uint32

// InitSpeaker starts the speaker at SpeakerSampleRate.
func InitSpeaker() error { return StartSpeaker(SpeakerSampleRate) }

// StartSpeaker starts I2S1 TX (BCK=G34, WS=G33, DOUT=G13, no MCLK, mono) at
// rate and enables the AW88298. The microphone must be stopped (shared I2S1).
// ref: M5Unified src/M5Unified.inl (CoreS3 spk_cfg: pin_bck 34, pin_ws 33, pin_data_out 13, i2s_port I2S_NUM_1)
func StartSpeaker(rate uint32) error {
	speakerRate = rate
	i2s.I2S1TX.Configure(i2s.Config{
		SampleRate: rate,
		BCK:        I2S_BCK,
		WS:         I2S_WS,
		DataOut:    I2S_DATA_OUT,
		MCLK:       machine.NoPin,
		Mono:       true,
	})
	i2s.I2S1TX.Start() // clocks running (silence) before the amp is enabled

	// AW88298 RST (AW9523B P0_2) high. ref: M5Unified _core_s3_aw9523_bit(0x02, 0b00000100, true)
	if err := IOExp.SetPins(aw9523b.Port0, P0_AW88298_RST, true); err != nil {
		return err
	}
	Amp = aw88298.New(I2C)
	return Amp.Enable(rate)
}

// StopSpeaker disables the amplifier and stops I2S TX.
// ref: M5Unified _speaker_enabled_cb_cores3 (disabled: 0x04 = 0x4000, AW9523B P0_2 low)
func StopSpeaker() {
	Amp.Disable()
	IOExp.SetPins(aw9523b.Port0, P0_AW88298_RST, false)
	i2s.I2S1TX.Stop()
}

// PlayPCM plays mono 16 bit samples at the speaker rate and returns when
// they have been played.
func PlayPCM(samples []int16) {
	i2s.I2S1TX.Write(samples)
	i2s.I2S1TX.Flush()
}

// toneWave is M5Unified's default tone wave (one sine period, unsigned 8 bit).
// ref: M5Unified src/utility/Speaker_Class.inl (_default_tone_wav)
var toneWave = [16]uint8{177, 219, 246, 255, 246, 219, 177, 128, 79, 37, 10, 1, 10, 37, 79, 128}

// ToneAmplitude scales toneWave (±127) to 16 bit.
// TODO(verify): M5Unified's level depends on master volume (64), channel
// volume and magnification (4); this is an approximation (about -12 dBFS).
var ToneAmplitude int32 = 64

var toneBuf [256]int16

// Tone plays a tone of freq Hz for ms milliseconds and returns when it has
// been played (M5.Speaker.tone(freq, ms) followed by the playback time).
func Tone(freq float32, ms uint32) {
	total := int(speakerRate * ms / 1000)
	step := freq * 16 / float32(speakerRate) // table entries per output sample
	phase := float32(0)
	for total > 0 {
		n := len(toneBuf)
		if n > total {
			n = total
		}
		for i := 0; i < n; i++ {
			i0 := int(phase)
			f := phase - float32(i0)
			a := int32(toneWave[i0&15]) - 128
			b := int32(toneWave[(i0+1)&15]) - 128
			v := float32(a) + (float32(b-a) * f)
			toneBuf[i] = int16(int32(v) * ToneAmplitude)
			phase += step
			if phase >= 16 {
				phase -= 16
			}
		}
		i2s.I2S1TX.Write(toneBuf[:n])
		total -= n
	}
	i2s.I2S1TX.Flush()
}
