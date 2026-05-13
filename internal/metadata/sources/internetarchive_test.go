package sources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newInternetArchiveFake(t *testing.T) (*httptest.Server, *InternetArchive) {
	meta := loadFixture(t, "internetarchive_metadata.json")
	search := loadFixture(t, "internetarchive_search.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/advancedsearch.php" && r.URL.Query().Get("q") != "":
			_, _ = w.Write(search)
		case r.URL.Path == "/metadata/frankenstein00mary":
			_, _ = w.Write(meta)
		case strings.HasPrefix(r.URL.Path, "/metadata/"):
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	ia := NewInternetArchiveAt(srv.URL, "test")
	ia.http.Client = srv.Client()
	return srv, ia
}

func TestInternetArchive_GetByIdentifier(t *testing.T) {
	srv, ia := newInternetArchiveFake(t)
	defer srv.Close()

	c, err := ia.Get(context.Background(), "frankenstein00mary", "us")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("nil candidate")
	}
	if c.Source != "internetarchive" {
		t.Errorf("source %q", c.Source)
	}
	if c.ExternalID != "frankenstein00mary" {
		t.Errorf("external id %q", c.ExternalID)
	}
	if c.Title != "Frankenstein; Or, The Modern Prometheus" {
		t.Errorf("title %q", c.Title)
	}
	// Creator is a JSON string in the fixture; must be wrapped into [].
	if len(c.Authors) != 1 || c.Authors[0] != "Shelley, Mary Wollstonecraft" {
		t.Errorf("authors %v (expected single author from string field)", c.Authors)
	}
	// Publisher is an array in the fixture; first element wins.
	if c.Publisher != "Lackington, Hughes, Harding, Mavor, and Jones" {
		t.Errorf("publisher %q", c.Publisher)
	}
	// Language is a string in the fixture.
	if c.Language != "English" {
		t.Errorf("language %q", c.Language)
	}
	// Date "1818-01-01" → year 1818.
	if c.PublishedAt != "1818" {
		t.Errorf("published_at %q (want year extracted from date)", c.PublishedAt)
	}
	// Description contains <p>/<br/>/<b> tags and &amp; entity; must be stripped & decoded.
	if strings.Contains(c.Description, "<") || strings.Contains(c.Description, "&amp;") {
		t.Errorf("description not cleaned: %q", c.Description)
	}
	if !strings.Contains(c.Description, "anonymously") || !strings.Contains(c.Description, "& widely") {
		t.Errorf("description content lost during cleaning: %q", c.Description)
	}
	// ISBN array — first element used.
	if c.ISBN != "9780000000000" {
		t.Errorf("isbn %q", c.ISBN)
	}
	// CoverURL is always computed from identifier; must point at services/img.
	if !strings.HasSuffix(c.CoverURL, "/services/img/frankenstein00mary") {
		t.Errorf("cover url %q", c.CoverURL)
	}
	if c.Region != "us" {
		t.Errorf("region %q", c.Region)
	}
	if len(c.Raw) == 0 {
		t.Error("expected Raw to be populated")
	}
}

func TestInternetArchive_GetMissing(t *testing.T) {
	srv, ia := newInternetArchiveFake(t)
	defer srv.Close()

	c, err := ia.Get(context.Background(), "no-such-identifier-xyz", "us")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if c != nil {
		t.Errorf("expected nil candidate, got %+v", c)
	}
}

func TestInternetArchive_GetEmptyReturnsNil(t *testing.T) {
	// No server: empty identifier must short-circuit before any HTTP call.
	ia := NewInternetArchiveAt("http://invalid.example.invalid", "test")
	c, err := ia.Get(context.Background(), "   ", "us")
	if err != nil {
		t.Fatalf("expected no error for empty id, got %v", err)
	}
	if c != nil {
		t.Errorf("expected nil candidate for empty id, got %+v", c)
	}
}

func TestInternetArchive_SearchByText(t *testing.T) {
	srv, ia := newInternetArchiveFake(t)
	defer srv.Close()

	cs, err := ia.Search(context.Background(), "frankenstein", "us")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("got %d candidates, want 2", len(cs))
	}

	// First doc: creator is an array with a duplicate; must dedupe to one entry.
	// Language is an array; first element used.
	first := cs[0]
	if first.Source != "internetarchive" {
		t.Errorf("source %q", first.Source)
	}
	if first.ExternalID != "frankenstein00mary" {
		t.Errorf("external id %q", first.ExternalID)
	}
	if first.Title != "Frankenstein; Or, The Modern Prometheus" {
		t.Errorf("title %q", first.Title)
	}
	if len(first.Authors) != 1 || first.Authors[0] != "Shelley, Mary Wollstonecraft" {
		t.Errorf("authors %v (expected dedupe on array creator)", first.Authors)
	}
	if first.Language != "English" {
		t.Errorf("language %q (expected first element of array)", first.Language)
	}
	if first.PublishedAt != "1818" {
		t.Errorf("published_at %q", first.PublishedAt)
	}
	if !strings.HasSuffix(first.CoverURL, "/services/img/frankenstein00mary") {
		t.Errorf("cover url %q", first.CoverURL)
	}

	// Second doc: creator is a string, language is a string, description is
	// an array of two strings (must be joined and cleaned), isbn is an array.
	second := cs[1]
	if second.ExternalID != "projecthailmary00weir" {
		t.Errorf("second external id %q", second.ExternalID)
	}
	if len(second.Authors) != 1 || second.Authors[0] != "Andy Weir" {
		t.Errorf("second authors %v (expected single from string field)", second.Authors)
	}
	if second.Language != "English" {
		t.Errorf("second language %q (expected string field)", second.Language)
	}
	if second.ISBN != "9780593135204" {
		t.Errorf("second isbn %q", second.ISBN)
	}
	if !strings.Contains(second.Description, "astronaut") || !strings.Contains(second.Description, "Hardcover") {
		t.Errorf("second description %q (expected array values joined)", second.Description)
	}
}
