package cores3

import (
	"machine"

	_ "esp32-s3-lite/internal/libcwrap" // malloc for fatfs (C)

	"tinygo.org/x/drivers/sdcard"
	"tinygo.org/x/tinyfs/fatfs"
)

// SDFrequency is the SPI clock used for card access after initialization.
// ref: https://docs.m5stack.com/ja/arduino/m5cores3/sdcard (SD.begin(SD_SPI_CS_PIN, SPI, 25000000))
const SDFrequency = 25 * machine.MHz

var (
	// SD is the microSD card, ready after InitSD.
	SD sdcard.Device
	// FS is the FAT file system on the card, mounted by InitSD.
	FS *fatfs.FATFS

	sdBlock sdBlockDevice
)

// GPIO35 is the LCD D/C line and the SD MISO line at the same time. M5GFX
// switches its role whenever the LCD chip select changes:
//
//	CS high (LCD idle): GPIO35 output disabled, routed to FSPIQ (MISO)
//	CS low  (LCD busy): GPIO35 is a GPIO output (D/C)
//
// ref: M5GFX src/M5GFX.cpp (Panel_M5StackCoreS3_T::cs_control)
//
// Here the switch is done around every SD block access instead (sdBegin /
// sdEnd), so the LCD side keeps a plain output pin and needs no hook. The SPI
// bus is also reconfigured, because the LCD runs at 40 MHz and the card at
// 25 MHz.

// sdBegin hands the bus to the SD card: LCD deselected, GPIO35 as MISO.
func sdBegin(freq uint32) {
	LCD_CS.High()
	// SPI.Configure routes FSPIQ from GPIO35 and makes the pin an input.
	SPI.Configure(machine.SPIConfig{
		Frequency: freq,
		SCK:       SD_SCK,
		SDO:       SD_MOSI,
		SDI:       SD_MISO,
		Mode:      0,
	})
}

// sdEnd hands the bus back to the LCD: 40 MHz, GPIO35 as D/C output.
func sdEnd() {
	SD_CS.High()
	SPI.Configure(machine.SPIConfig{
		Frequency: LCDFrequency,
		SCK:       LCD_SCK,
		SDO:       LCD_MOSI,
		SDI:       machine.NoPin,
		Mode:      0,
	})
	LCD_DC.Configure(machine.PinConfig{Mode: machine.PinOutput})
	LCD_DC.High()
}

// sdBlockDevice wraps the card so that every access switches the bus.
type sdBlockDevice struct{ dev *sdcard.Device }

func (b *sdBlockDevice) ReadAt(p []byte, off int64) (int, error) {
	sdBegin(SDFrequency)
	defer sdEnd()
	return b.dev.ReadAt(p, off)
}

func (b *sdBlockDevice) WriteAt(p []byte, off int64) (int, error) {
	sdBegin(SDFrequency)
	defer sdEnd()
	return b.dev.WriteAt(p, off)
}

func (b *sdBlockDevice) Size() int64           { return b.dev.Size() }
func (b *sdBlockDevice) WriteBlockSize() int64 { return b.dev.WriteBlockSize() }
func (b *sdBlockDevice) EraseBlockSize() int64 { return b.dev.EraseBlockSize() }
func (b *sdBlockDevice) EraseBlocks(start, n int64) error {
	sdBegin(SDFrequency)
	defer sdEnd()
	return b.dev.EraseBlocks(start, n)
}

// InitSD initializes the card and mounts the FAT file system. The card must
// be FAT12/16/32 (tinyfs fatfs is built without exFAT). The file system is
// never formatted.
// Init and InitDisplay must have been called before.
func InitSD() error {
	sdBegin(SDFrequency)
	SD = sdcard.New(SPI, SD_SCK, SD_MOSI, SD_MISO, SD_CS)
	err := SD.Configure() // reconfigures the bus itself (250 kHz, then 4 MHz)
	sdEnd()
	if err != nil {
		return err
	}
	sdBlock.dev = &SD
	FS = fatfs.New(&sdBlock)
	FS.Configure(&fatfs.Config{SectorSize: 512})
	return FS.Mount()
}
