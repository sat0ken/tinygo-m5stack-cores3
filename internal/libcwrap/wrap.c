// The esp32s3 TinyGo target links with --wrap=malloc/calloc/free (for the
// Wi-Fi blob support in espradio). C code that calls malloc (here: tinyfs
// fatfs) then needs __wrap_malloc etc. When espradio is not linked, forward
// them to the TinyGo runtime implementations (__real_*).
#include <stddef.h>

void *__real_malloc(size_t size);
void *__real_calloc(size_t nmemb, size_t size);
void __real_free(void *ptr);

void *__wrap_malloc(size_t size) { return __real_malloc(size); }
void *__wrap_calloc(size_t nmemb, size_t size) { return __real_calloc(nmemb, size); }
void __wrap_free(void *ptr) { __real_free(ptr); }
