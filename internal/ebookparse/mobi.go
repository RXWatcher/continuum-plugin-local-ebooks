package ebookparse

// ParseMOBI extracts metadata from MOBI/AZW/AZW3 files. Implemented in Task 9.
func ParseMOBI(path, ext string) (Parsed, error) { return Parsed{}, ErrUnsupportedFormat }
