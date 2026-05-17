package ebookparse

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// TestParseMOBI_TruncatedRecord0DoesNotPanic guards the slice-bounds panic:
// a ~100-byte file whose record-0 offset leaves <24 bytes for the MOBI
// header used to panic (data[rec0+20:rec0+24] out of range) and crash the
// whole scan. It must return an error instead.
func TestParseMOBI_TruncatedRecord0DoesNotPanic(t *testing.T) {
	data := make([]byte, 100)
	binary.BigEndian.PutUint16(data[76:78], 1)  // numRecords = 1
	binary.BigEndian.PutUint32(data[78:82], 79) // rec0Offset = len-21
	copy(data[95:99], []byte("MOBI"))           // sig at rec0Offset+16

	fp := filepath.Join(t.TempDir(), "x.mobi")
	if err := os.WriteFile(fp, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseMOBI(fp, ".mobi"); err == nil {
		t.Fatal("expected error for truncated record-0 header, got nil")
	}
}
