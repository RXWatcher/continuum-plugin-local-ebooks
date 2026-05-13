package sources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newHardcoverFake(t *testing.T) (*httptest.Server, *Hardcover) {
	t.Helper()
	book := loadFixture(t, "hardcover_book.json")
	search := loadFixture(t, "hardcover_search.json")
	missing := []byte(`{"data":{"books_by_pk":null}}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		// Distinguish book vs search by inspecting the request body.
		// Both POST to /graphql; we serve the right fixture based on
		// whether the request body contains "books_by_pk" (Get) or "books(" (Search).
		var buf [4096]byte
		n, _ := r.Body.Read(buf[:])
		body := string(buf[:n])

		w.Header().Set("Content-Type", "application/json")
		switch {
		case contains(body, "books_by_pk") && contains(body, `"id":97844`):
			w.Write(book)
		case contains(body, "books_by_pk") && contains(body, `"id":0`):
			// non-existent id
			w.Write(missing)
		case contains(body, "books_by_pk"):
			w.Write(missing)
		default:
			w.Write(search)
		}
	}))

	h := NewHardcoverAt(srv.URL, "test-api-key", "test")
	h.http.Client = srv.Client()
	return srv, h
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

func TestHardcover_GetByID(t *testing.T) {
	srv, h := newHardcoverFake(t)
	defer srv.Close()

	c, err := h.Get(context.Background(), "97844", "us")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("nil candidate")
	}
	if c.Title != "Project Hail Mary" {
		t.Errorf("title %q", c.Title)
	}
	if c.Source != hardcoverID {
		t.Errorf("source %q", c.Source)
	}
	if c.ExternalID != "97844" {
		t.Errorf("externalID %q", c.ExternalID)
	}
	if len(c.Authors) == 0 || c.Authors[0] != "Andy Weir" {
		t.Errorf("authors %v", c.Authors)
	}
	if c.ISBN != "9780593135204" {
		t.Errorf("isbn %q", c.ISBN)
	}
	if c.CoverURL != "https://example/cover.jpg" {
		t.Errorf("coverURL %q", c.CoverURL)
	}
	if c.PageCount != 476 {
		t.Errorf("pageCount %d", c.PageCount)
	}
	if c.PublishedAt != "2021-05-04" {
		t.Errorf("publishedAt %q", c.PublishedAt)
	}
}

func TestHardcover_GetMissing(t *testing.T) {
	missing := []byte(`{"data":{"books_by_pk":null}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(missing)
	}))
	defer srv.Close()

	h := NewHardcoverAt(srv.URL, "test-api-key", "test")
	h.http.Client = srv.Client()

	c, err := h.Get(context.Background(), "99999", "us")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got err=%v c=%v", err, c)
	}
	if c != nil {
		t.Errorf("expected nil candidate")
	}
}

func TestHardcover_SearchByText(t *testing.T) {
	srv, h := newHardcoverFake(t)
	defer srv.Close()

	cs, err := h.Search(context.Background(), "Project Hail Mary", "us")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) < 1 {
		t.Fatalf("expected >=1 candidate, got %d", len(cs))
	}
	if cs[0].Title != "Project Hail Mary" {
		t.Errorf("title %q", cs[0].Title)
	}
	if cs[0].Source != hardcoverID {
		t.Errorf("source %q", cs[0].Source)
	}
}

func TestHardcover_DisabledWhenNoKey(t *testing.T) {
	h := NewHardcover("", "test")
	if h.Enabled(map[string]bool{hardcoverID: true}) {
		t.Error("expected Enabled=false when apiKey is empty")
	}
}

func TestHardcover_NonNumericGetReturnsNil(t *testing.T) {
	// No server needed — non-numeric IDs are rejected before any HTTP call.
	h := NewHardcover("test-api-key", "test")

	c, err := h.Get(context.Background(), "not-a-number", "us")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if c != nil {
		t.Errorf("expected nil candidate for non-numeric id, got %+v", c)
	}
}
