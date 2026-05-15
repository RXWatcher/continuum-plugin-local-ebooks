package ebookparse

import (
	"errors"
	"testing"
)

func TestParse_UnsupportedFormat(t *testing.T) {
	_, err := Parse("/tmp/something.txt")
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestIsSupported(t *testing.T) {
	cases := map[string]bool{
		"book.epub":      true,
		"book.EPUB":      true,
		"book.pdf":       true,
		"book.mobi":      true,
		"book.azw":       true,
		"book.azw3":      true,
		"book.fb2":       true,
		"book.txt":       false,
		"book":           false,
		"no/dot/file":    false,
		"/abs/path.epub": true,
	}
	for path, want := range cases {
		if got := IsSupported(path); got != want {
			t.Errorf("IsSupported(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestExtOf(t *testing.T) {
	cases := map[string]string{
		"book.epub":         ".epub",
		"/path/to/book.pdf": ".pdf",
		"book":              "",
		"book.tar.gz":       ".gz",
		"/path/no.dot/file": "",
	}
	for path, want := range cases {
		if got := extOf(path); got != want {
			t.Errorf("extOf(%q) = %q, want %q", path, got, want)
		}
	}
}
