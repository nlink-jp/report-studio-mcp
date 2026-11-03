package pdf

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework PDFKit
#include <stdlib.h>
char *rs_pdf_text(const unsigned char *bytes, long n, int *pages);
*/
import "C"

import (
	"errors"
	"unsafe"
)

// ExtractText reads a PDF back through PDFKit: its text and page count.
func ExtractText(pdf []byte) (string, int, error) {
	if len(pdf) == 0 {
		return "", 0, errors.New("empty PDF")
	}
	var pages C.int
	s := C.rs_pdf_text((*C.uchar)(unsafe.Pointer(&pdf[0])), C.long(len(pdf)), &pages)
	if s == nil {
		return "", 0, errors.New("PDFKit cannot read the bytes")
	}
	defer C.free(unsafe.Pointer(s))
	return C.GoString(s), int(pages), nil
}
