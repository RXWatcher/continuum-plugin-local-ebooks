package ebookparse

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeTestEPUB(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "sample.epub")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)

	w, _ := zw.Create("META-INF/container.xml")
	w.Write([]byte(`<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`))

	w, _ = zw.Create("OEBPS/content.opf")
	w.Write([]byte(`<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>Project Hail Mary</dc:title>
    <dc:creator>Andy Weir</dc:creator>
    <dc:description>A lone astronaut.</dc:description>
    <dc:publisher>Ballantine</dc:publisher>
    <dc:date>2021-05-04</dc:date>
    <dc:language>en</dc:language>
    <dc:subject>Science Fiction</dc:subject>
    <dc:identifier opf:scheme="ISBN">9780593135204</dc:identifier>
    <meta name="cover" content="cover-id"/>
    <meta name="calibre:series" content="Hail Mary"/>
    <meta name="calibre:series_index" content="1"/>
  </metadata>
  <manifest>
    <item id="cover-id" href="cover.jpg" media-type="image/jpeg"/>
  </manifest>
</package>`))

	w, _ = zw.Create("OEBPS/cover.jpg")
	w.Write([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F'})

	zw.Close()
	return p
}

func TestParseEPUB_HappyPath(t *testing.T) {
	dir := t.TempDir()
	p := writeTestEPUB(t, dir)
	parsed, err := ParseEPUB(p)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Format != "epub" {
		t.Errorf("format %q", parsed.Format)
	}
	if parsed.Title != "Project Hail Mary" {
		t.Errorf("title %q", parsed.Title)
	}
	if len(parsed.Authors) != 1 || parsed.Authors[0] != "Andy Weir" {
		t.Errorf("authors %v", parsed.Authors)
	}
	if parsed.Publisher != "Ballantine" {
		t.Errorf("publisher %q", parsed.Publisher)
	}
	if parsed.Language != "en" {
		t.Errorf("language %q", parsed.Language)
	}
	if parsed.ISBN != "9780593135204" {
		t.Errorf("isbn %q", parsed.ISBN)
	}
	if parsed.PublishedAt.Year() != 2021 {
		t.Errorf("year %d", parsed.PublishedAt.Year())
	}
	if parsed.Series != "Hail Mary" || parsed.SeriesPos != "1" {
		t.Errorf("series %q %q", parsed.Series, parsed.SeriesPos)
	}
	if parsed.Cover == nil {
		t.Fatal("expected cover")
	}
	if parsed.Cover.ContentType != "image/jpeg" {
		t.Errorf("cover ct %q", parsed.Cover.ContentType)
	}
	if !bytes.HasPrefix(parsed.Cover.Bytes, []byte{0xff, 0xd8, 0xff, 0xe0}) {
		t.Errorf("cover bytes don't look like JPEG")
	}
}

func TestParseEPUB_MissingFile(t *testing.T) {
	_, err := ParseEPUB("/nonexistent.epub")
	if err == nil {
		t.Error("expected error")
	}
}
