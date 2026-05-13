package sources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newAnnasArchiveFake serves the search fixture, a happy-path detail page
// for the canonical MD5, and an audiobook detail page for a second MD5.
// Any other /md5/... path returns 404 so the not-found path is exercised.
func newAnnasArchiveFake(t *testing.T) (*httptest.Server, *AnnasArchive) {
	t.Helper()
	book := loadFixture(t, "annasarchive_book.html")
	audio := loadFixture(t, "annasarchive_audiobook.html")
	search := loadFixture(t, "annasarchive_search.html")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/md5/a1b2c3d4e5f67890abcdef1234567890"):
			w.Header().Set("Content-Type", "text/html")
			w.Write(book)
		case strings.HasPrefix(r.URL.Path, "/md5/c3d4e5f67890abcdef1234567890abcd"):
			w.Header().Set("Content-Type", "text/html")
			w.Write(audio)
		case strings.HasPrefix(r.URL.Path, "/md5/"):
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/search":
			w.Header().Set("Content-Type", "text/html")
			w.Write(search)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	a := NewAnnasArchiveAt(srv.URL, "test-agent")
	a.http.Client = srv.Client()
	return srv, a
}

// TestAnnasArchive_GetByMD5 verifies the happy path: every field the detail
// parser populates is asserted so any selector regression is visible.
func TestAnnasArchive_GetByMD5(t *testing.T) {
	srv, a := newAnnasArchiveFake(t)
	defer srv.Close()

	c, err := a.Get(context.Background(), "a1b2c3d4e5f67890abcdef1234567890", "us")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil candidate")
	}
	if c.Title != "Project Hail Mary" {
		t.Errorf("title: got %q, want %q", c.Title, "Project Hail Mary")
	}
	if c.Source != annasArchiveID {
		t.Errorf("source: got %q, want %q", c.Source, annasArchiveID)
	}
	if c.ExternalID != "a1b2c3d4e5f67890abcdef1234567890" {
		t.Errorf("external_id: got %q, want %q", c.ExternalID, "a1b2c3d4e5f67890abcdef1234567890")
	}
	if len(c.Authors) != 1 || c.Authors[0] != "Andy Weir" {
		t.Errorf("authors: got %v, want [Andy Weir]", c.Authors)
	}
	if c.ISBN != "9780593135204" {
		t.Errorf("isbn: got %q, want %q", c.ISBN, "9780593135204")
	}
	if c.Publisher != "Ballantine Books" {
		t.Errorf("publisher: got %q, want %q", c.Publisher, "Ballantine Books")
	}
	if c.PublishedAt != "2021" {
		t.Errorf("published_at: got %q, want %q", c.PublishedAt, "2021")
	}
	// Language: detail label captures "English [en]" → first 2 lower chars = "en".
	if c.Language != "en" {
		t.Errorf("language: got %q, want %q", c.Language, "en")
	}
	if c.PageCount != 476 {
		t.Errorf("page_count: got %d, want 476", c.PageCount)
	}
	if c.CoverURL == "" || !strings.HasSuffix(c.CoverURL, "/cover/a1b2c3d4.jpg") {
		t.Errorf("cover URL not resolved correctly: %q", c.CoverURL)
	}
	if c.Description == "" || !strings.Contains(c.Description, "Ryland Grace") {
		t.Errorf("expected non-empty description containing 'Ryland Grace', got %q", c.Description)
	}
	// Numeric-entity decoding: &#39; in the fixture description must be rendered as '.
	if strings.Contains(c.Description, "&#39;") {
		t.Errorf("description still contains raw numeric entity: %q", c.Description)
	}
	if c.Region != "us" {
		t.Errorf("region: got %q, want %q", c.Region, "us")
	}
}

// TestAnnasArchive_GetMissing verifies that a 404 surfaces as ErrNotFound.
func TestAnnasArchive_GetMissing(t *testing.T) {
	srv, a := newAnnasArchiveFake(t)
	defer srv.Close()

	c, err := a.Get(context.Background(), "ffffffffffffffffffffffffffffffff", "us")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got err=%v c=%v", err, c)
	}
	if c != nil {
		t.Error("expected nil candidate on not-found")
	}
}

// TestAnnasArchive_GetNonMD5ReturnsNil verifies that a non-MD5 input (an
// ISBN-13, for instance) short-circuits to (nil, nil) without a network
// call. The fake's default 404 means any HTTP request would fail the
// "nil error" assertion.
func TestAnnasArchive_GetNonMD5ReturnsNil(t *testing.T) {
	srv, a := newAnnasArchiveFake(t)
	defer srv.Close()

	c, err := a.Get(context.Background(), "9780593135204", "us")
	if err != nil {
		t.Errorf("expected nil error for non-MD5 id, got %v", err)
	}
	if c != nil {
		t.Errorf("expected nil candidate for non-MD5 id, got %+v", c)
	}
}

// TestAnnasArchive_SearchFiltersAudiobookFormats is the marquee assertion
// for this source: the search fixture contains an .m4b row that MUST be
// excluded, and at least one .epub/.pdf row that MUST be included. The
// "unknown" author row (epub) also confirms the unknown-sentinel skip
// behaviour and that filtering happens by extension, not by author.
func TestAnnasArchive_SearchFiltersAudiobookFormats(t *testing.T) {
	srv, a := newAnnasArchiveFake(t)
	defer srv.Close()

	cs, err := a.Search(context.Background(), "project hail mary", "us")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cs) == 0 {
		t.Fatal("expected at least one candidate")
	}

	// The .m4b row's MD5 must NOT appear in results.
	const audiobookMD5 = "c3d4e5f67890abcdef1234567890abcd"
	for _, c := range cs {
		if c.ExternalID == audiobookMD5 {
			t.Errorf("audiobook row (m4b) was not filtered out: %+v", c)
		}
	}

	// At least one ebook row (the .epub Project Hail Mary) must be present
	// with its full populated metadata.
	var ebook *struct {
		Title, ISBN, Publisher, PublishedAt, Language, ExternalID string
		Authors                                                   []string
	}
	for i := range cs {
		c := cs[i]
		if c.ExternalID == "a1b2c3d4e5f67890abcdef1234567890" {
			ebook = &struct {
				Title, ISBN, Publisher, PublishedAt, Language, ExternalID string
				Authors                                                   []string
			}{c.Title, c.ISBN, c.Publisher, c.PublishedAt, c.Language, c.ExternalID, c.Authors}
			break
		}
	}
	if ebook == nil {
		t.Fatal("expected the .epub Project Hail Mary row in results")
	}
	if ebook.Title != "Project Hail Mary" {
		t.Errorf("title: got %q, want %q", ebook.Title, "Project Hail Mary")
	}
	if ebook.ISBN != "9780593135204" {
		t.Errorf("isbn: got %q, want %q", ebook.ISBN, "9780593135204")
	}
	if ebook.Publisher != "Ballantine Books" {
		t.Errorf("publisher: got %q, want %q", ebook.Publisher, "Ballantine Books")
	}
	if ebook.PublishedAt != "2021" {
		t.Errorf("published_at: got %q, want %q", ebook.PublishedAt, "2021")
	}
	if ebook.Language != "en" {
		t.Errorf("language: got %q, want %q", ebook.Language, "en")
	}
	if len(ebook.Authors) != 1 || ebook.Authors[0] != "Andy Weir" {
		t.Errorf("authors: got %v, want [Andy Weir]", ebook.Authors)
	}

	// The "unknown" author row's epub must still be in results (filter is
	// by extension, not by author); its Authors field must be empty.
	const unknownMD5 = "d4e5f67890abcdef1234567890abcdef"
	var unknownRow *struct {
		Authors []string
	}
	for i := range cs {
		if cs[i].ExternalID == unknownMD5 {
			unknownRow = &struct{ Authors []string }{cs[i].Authors}
			break
		}
	}
	if unknownRow == nil {
		t.Error("expected the unknown-author .epub row to be present")
	} else if len(unknownRow.Authors) != 0 {
		t.Errorf("expected empty Authors for the unknown-sentinel row, got %v", unknownRow.Authors)
	}
}

// TestAnnasArchive_GetAudiobookFormatReturnsNotFound documents the Get-side
// of the format filter: a successfully-fetched detail page whose Extension
// is m4b (or any non-ebook format) is reported as ErrNotFound, matching the
// "not a book per our definition" signal callers get from a 404.
func TestAnnasArchive_GetAudiobookFormatReturnsNotFound(t *testing.T) {
	srv, a := newAnnasArchiveFake(t)
	defer srv.Close()

	c, err := a.Get(context.Background(), "c3d4e5f67890abcdef1234567890abcd", "us")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for m4b detail page, got err=%v c=%v", err, c)
	}
	if c != nil {
		t.Error("expected nil candidate for audiobook format")
	}
}
