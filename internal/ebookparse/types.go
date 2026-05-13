// Package ebookparse extracts inline metadata from ebook files.
// One parser per format; the dispatcher routes by extension.
package ebookparse

import "time"

// Parsed is the format-agnostic shape returned by every parser.
// Empty fields mean "unknown / not present in this file".
type Parsed struct {
	Format      string // "epub" / "pdf" / "mobi" / "azw" / "azw3" / "fb2"
	Title       string
	Authors     []string
	Description string
	Publisher   string
	PublishedAt time.Time
	Language    string
	ISBN        string
	ASIN        string
	Series      string
	SeriesPos   string
	Genres      []string
	PageCount   int
	Cover       *Cover // nil if no embedded cover
}

// Cover is an embedded cover image extracted from the file.
type Cover struct {
	ContentType string
	Bytes       []byte
}
