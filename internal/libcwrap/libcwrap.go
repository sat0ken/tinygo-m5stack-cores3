// Package libcwrap provides __wrap_malloc / __wrap_calloc / __wrap_free for
// programs that use C code (tinyfs fatfs) on the esp32s3 target without
// espradio. Import it for its side effect. Do not import it together with
// espradio (duplicate symbols).
//
// ref: TinyGo targets/esp32s3.json ("--wrap=malloc", "--wrap=calloc", "--wrap=free")
package libcwrap

import "C"
