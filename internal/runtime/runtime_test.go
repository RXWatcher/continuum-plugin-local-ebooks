package runtime

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestConfigRedaction(t *testing.T) {
	cfg := Config{
		DatabaseURL:           "postgres://u:sup3rsecret@db/x",
		StreamSigningSecret:   "STREAMSECRET",
		GoogleBooksAPIKey:     "GKEY",
		ISBNdbAPIKey:          "IKEY",
		HardcoverAPIKey:       "HKEY",
		MetadataDefaultRegion: "us",
	}
	leaks := []string{"sup3rsecret", "STREAMSECRET", "GKEY", "IKEY", "HKEY"}
	if s := cfg.String(); containsAny(s, leaks) {
		t.Fatalf("String leaked a secret: %s", s)
	}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("cfg", "config", cfg)
	out := buf.String()
	if containsAny(out, leaks) {
		t.Fatalf("slog leaked a secret: %s", out)
	}
	if !strings.Contains(out, "us") {
		t.Fatalf("redaction hid the non-secret region: %s", out)
	}
}

func containsAny(s string, subs []string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

func TestSnapshot_SlicesIsolated(t *testing.T) {
	s := New(nil, func(Config) error { return nil })
	s.mu.Lock()
	s.cfg = Config{
		LibraryPaths:           []string{"/a"},
		MetadataSourcesEnabled: []string{"openlibrary"},
		Libraries:              []LibraryConfig{{Path: "/a", Name: "A"}},
	}
	s.mu.Unlock()

	snap := s.Snapshot()
	snap.LibraryPaths[0] = "MUT"
	snap.MetadataSourcesEnabled[0] = "MUT"
	snap.Libraries[0].Path = "MUT"

	again := s.Snapshot()
	if again.LibraryPaths[0] != "/a" || again.MetadataSourcesEnabled[0] != "openlibrary" || again.Libraries[0].Path != "/a" {
		t.Fatalf("Snapshot aliases backing arrays: %+v", again)
	}
}
