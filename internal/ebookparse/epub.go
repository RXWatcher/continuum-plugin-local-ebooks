package ebookparse

// ParseEPUB extracts metadata from an EPUB file. Implemented in Task 7.
func ParseEPUB(path string) (Parsed, error) { return Parsed{}, ErrUnsupportedFormat }
