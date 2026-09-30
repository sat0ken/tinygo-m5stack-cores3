package ili9342

import (
	"machine"
	"time"
)

// ProbeResult holds the raw values read while probing, for logging.
type ProbeResult struct {
	Variant Variant
	RDDID   uint8    // first byte of RDDID (04h); 0xE3 on an ILI9342
	Keys    [4]uint8 // DDh, CBh read-backs (E keys) and D3h ID4 bytes 2,3 (C keys)
}

// ProbeVariant tells an ILI9342C from an ILI9342E, following M5GFX.
//
// The ILI9342 is read back over its bidirectional SDA line (3-wire read), which
// the TinyGo SPI peripheral driver does not support, so the probe bit-bangs the
// four pins as GPIO. Call it before configuring the SPI bus (or reconfigure the
// bus afterwards): the pins are left as plain GPIO outputs.
//
// ref: M5GFX src/M5GFX.cpp (_read_panel_id, _probe_ili9342_variant, _identify_ili9342)
//
//   - write D9h = 00h, DDh = 01h, CBh = 1Ch
//   - read DDh and CBh through D9h (10h | index): the E reads 01h / 1Ch back
//   - otherwise send the C EXTC key (C8h FF 93 42) and read D3h (ID4) index 2,3:
//     the C returns 93h 42h (compared as 13h 42h after the 7 bit normalisation)
//   - an inconclusive probe is repeated once after 1 ms; the caller falls back
//     to the C when it stays unknown.
func ProbeVariant(sck, sda, cs, dc machine.Pin) ProbeResult {
	b := bitbang{sck: sck, sda: sda, cs: cs, dc: dc}
	b.init()

	var res ProbeResult
	res.RDDID = b.readCommand(RDDID, 1, false)

	res.Variant = b.probe(&res.Keys)
	if res.Variant == VariantUnknown {
		time.Sleep(time.Millisecond)
		res.Variant = b.probe(&res.Keys)
	}
	return res
}

type bitbang struct {
	sck, sda, cs, dc machine.Pin
}

var spinSink uint32

// spin waits a few hundred ns: the ILI9342 serial read cycle is >= 150 ns.
func spin() {
	for i := 0; i < 40; i++ {
		spinSink++
	}
}

func (b *bitbang) init() {
	out := machine.PinConfig{Mode: machine.PinOutput}
	b.cs.Configure(out)
	b.cs.High()
	b.dc.Configure(out)
	b.dc.High()
	b.sck.Configure(out)
	b.sck.Low() // SPI mode 0
	b.sda.Configure(out)
	b.sda.Low()
}

func (b *bitbang) writeByte(v uint8) {
	for i := 0; i < 8; i++ {
		b.sda.Set(v&0x80 != 0)
		v <<= 1
		spin()
		b.sck.High()
		spin()
		b.sck.Low()
	}
}

// clock issues one clock and samples SDA on the rising edge (mode 0).
func (b *bitbang) clock() bool {
	spin()
	b.sck.High()
	spin()
	v := b.sda.Get()
	b.sck.Low()
	return v
}

// write8 sends cmd + one data byte in its own transaction closed with a NOP,
// like M5GFX does on a panel object that is not initialized yet.
func (b *bitbang) write(cmd uint8, data ...uint8) {
	b.cs.Low()
	b.dc.Low()
	b.writeByte(cmd)
	b.dc.High()
	for _, v := range data {
		b.writeByte(v)
	}
	b.dc.Low()
	b.writeByte(NOP)
	b.dc.High()
	b.cs.High()
}

// readCommand sends cmd, releases SDA, gives dummy clocks and reads one byte.
// With normalise, the M5GFX 7 bit normalisation for D9h reads is applied.
func (b *bitbang) readCommand(cmd uint8, dummy int, normalise bool) uint8 {
	b.cs.Low()
	b.dc.Low()
	b.writeByte(cmd)
	b.dc.High()
	b.sda.Configure(machine.PinConfig{Mode: machine.PinInput})
	for i := 0; i < dummy; i++ {
		b.clock()
	}
	var v uint8
	for i := 0; i < 8; i++ {
		v <<= 1
		if b.clock() {
			v |= 1
		}
	}
	b.cs.High()
	b.sda.Configure(machine.PinConfig{Mode: machine.PinOutput})
	b.sda.Low()
	if normalise {
		// ref: M5GFX _probe_ili9342_variant: "One dummy bit shifts the byte left
		// by one; normalise to the datasheet value." (v >> 1) & 0x7F
		v = (v >> 1) & 0x7F
	}
	return v
}

func (b *bitbang) readParam(cmd, index uint8) uint8 {
	b.write(0xD9, 0x10|index)
	return b.readCommand(cmd, 1, true)
}

func (b *bitbang) probe(keys *[4]uint8) Variant {
	b.write(0xD9, 0x00) // a leftover index would make the panel misread the parameters below
	b.write(0xDD, 0x01)
	b.write(0xCB, 0x1C)
	keys[0] = b.readParam(0xDD, 1)
	keys[1] = b.readParam(0xCB, 1)
	b.write(0xD9, 0x00)
	if keys[0] == 0x01 && keys[1] == 0x1C {
		return VariantE
	}
	b.write(0xC8, 0xFF, 0x93, 0x42)
	keys[2] = b.readParam(0xD3, 2)
	keys[3] = b.readParam(0xD3, 3)
	b.write(0xD9, 0x00)
	if keys[2] == 0x93&0x7F && keys[3] == 0x42 {
		return VariantC
	}
	return VariantUnknown
}
