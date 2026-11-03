// Text extraction through PDFKit — how the tests read back what was drawn, and
// what Preview and Spotlight see.
#import <Foundation/Foundation.h>
#import <PDFKit/PDFKit.h>
#include <stdlib.h>
#include <string.h>

// rs_pdf_text returns the document's text as malloc'd UTF-8 (NULL if the bytes
// are not a PDF) and its page count in *pages.
char *rs_pdf_text(const unsigned char *bytes, long n, int *pages) {
	@autoreleasepool {
		NSData *data = [NSData dataWithBytes:bytes length:(NSUInteger)n];
		PDFDocument *doc = [[PDFDocument alloc] initWithData:data];
		if (doc == nil) return NULL;
		*pages = (int)doc.pageCount;
		NSString *s = doc.string ?: @"";
		const char *u = s.UTF8String;
		return strdup(u ? u : "");
	}
}
