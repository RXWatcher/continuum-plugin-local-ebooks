package ebookparse

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

// ParseEPUB extracts metadata from an EPUB file.
func ParseEPUB(filePath string) (Parsed, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return Parsed{}, fmt.Errorf("epub: open: %w", err)
	}
	defer r.Close()

	opfPath, err := findOPFPath(&r.Reader)
	if err != nil {
		return Parsed{}, err
	}

	opfFile, err := openZipEntry(&r.Reader, opfPath)
	if err != nil {
		return Parsed{}, fmt.Errorf("epub: open opf: %w", err)
	}
	opfBytes, err := io.ReadAll(io.LimitReader(opfFile, 4<<20))
	opfFile.Close()
	if err != nil {
		return Parsed{}, fmt.Errorf("epub: read opf: %w", err)
	}

	var pkg opfPackage
	if err := xml.Unmarshal(opfBytes, &pkg); err != nil {
		return Parsed{}, fmt.Errorf("epub: parse opf: %w", err)
	}

	out := Parsed{
		Format:      "epub",
		Title:       firstNonEmpty(pkg.Metadata.Title),
		Description: firstNonEmpty(pkg.Metadata.Description),
		Publisher:   firstNonEmpty(pkg.Metadata.Publisher),
		Language:    firstNonEmpty(pkg.Metadata.Language),
	}
	for _, a := range pkg.Metadata.Creator {
		if s := strings.TrimSpace(a.Value); s != "" {
			out.Authors = append(out.Authors, s)
		}
	}
	for _, s := range pkg.Metadata.Subject {
		if v := strings.TrimSpace(s); v != "" {
			out.Genres = append(out.Genres, v)
		}
	}
	if d := firstNonEmpty(pkg.Metadata.Date); d != "" {
		if t, err := tryParseDate(d); err == nil {
			out.PublishedAt = t
		}
	}
	for _, id := range pkg.Metadata.Identifier {
		v := strings.TrimSpace(id.Value)
		scheme := strings.ToUpper(strings.TrimSpace(id.Scheme))
		if scheme == "ISBN" || looksLikeISBN(v) {
			out.ISBN = v
		}
		if scheme == "AMAZON" || scheme == "ASIN" || looksLikeASIN(v) {
			out.ASIN = v
		}
	}
	// Calibre series metadata
	for _, m := range pkg.Metadata.Meta {
		switch m.Name {
		case "calibre:series":
			out.Series = m.Content
		case "calibre:series_index":
			out.SeriesPos = m.Content
		}
	}

	// Cover extraction
	coverID := ""
	for _, m := range pkg.Metadata.Meta {
		if m.Name == "cover" {
			coverID = m.Content
			break
		}
	}
	coverHref := ""
	coverContentType := ""
	for _, item := range pkg.Manifest.Items {
		if (coverID != "" && item.ID == coverID) ||
			strings.Contains(item.Properties, "cover-image") {
			coverHref = item.Href
			coverContentType = item.MediaType
			break
		}
	}
	if coverHref != "" {
		coverPath := path.Join(path.Dir(opfPath), coverHref)
		if coverFile, err := openZipEntry(&r.Reader, coverPath); err == nil {
			body, err := io.ReadAll(io.LimitReader(coverFile, 5<<20))
			coverFile.Close()
			if err == nil && len(body) > 0 {
				if coverContentType == "" {
					coverContentType = "image/jpeg"
				}
				out.Cover = &Cover{ContentType: coverContentType, Bytes: body}
			}
		}
	}

	return out, nil
}

type containerXML struct {
	Rootfiles struct {
		Rootfile struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfile"`
	} `xml:"rootfiles"`
}

func findOPFPath(r *zip.Reader) (string, error) {
	f, err := openZipEntry(r, "META-INF/container.xml")
	if err != nil {
		return "", fmt.Errorf("epub: container.xml: %w", err)
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, 64<<10))
	if err != nil {
		return "", fmt.Errorf("epub: container.xml read: %w", err)
	}
	var c containerXML
	if err := xml.Unmarshal(body, &c); err != nil {
		return "", fmt.Errorf("epub: container.xml parse: %w", err)
	}
	if c.Rootfiles.Rootfile.FullPath == "" {
		return "", fmt.Errorf("epub: no rootfile in container.xml")
	}
	return c.Rootfiles.Rootfile.FullPath, nil
}

func openZipEntry(r *zip.Reader, name string) (io.ReadCloser, error) {
	for _, f := range r.File {
		if f.Name == name {
			return f.Open()
		}
	}
	return nil, fmt.Errorf("epub: entry not found: %s", name)
}

type opfPackage struct {
	Metadata opfMetadata `xml:"metadata"`
	Manifest opfManifest `xml:"manifest"`
}

type opfMetadata struct {
	Title       []string        `xml:"title"`
	Creator     []opfCreator    `xml:"creator"`
	Description []string        `xml:"description"`
	Publisher   []string        `xml:"publisher"`
	Date        []string        `xml:"date"`
	Language    []string        `xml:"language"`
	Subject     []string        `xml:"subject"`
	Identifier  []opfIdentifier `xml:"identifier"`
	Meta        []opfMeta       `xml:"meta"`
}

type opfCreator struct {
	Value string `xml:",chardata"`
}

type opfIdentifier struct {
	Scheme string `xml:"scheme,attr"`
	Value  string `xml:",chardata"`
}

type opfMeta struct {
	Name    string `xml:"name,attr"`
	Content string `xml:"content,attr"`
}

type opfManifest struct {
	Items []opfManifestItem `xml:"item"`
}

type opfManifestItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

func firstNonEmpty(ss []string) string {
	for _, s := range ss {
		if v := strings.TrimSpace(s); v != "" {
			return v
		}
	}
	return ""
}

// tryParseDate accepts YYYY-MM-DD, YYYY-MM, YYYY, or RFC3339.
func tryParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", "2006-01", "2006", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date: %q", s)
}

func looksLikeISBN(s string) bool {
	digits := 0
	for _, c := range s {
		if (c >= '0' && c <= '9') || c == 'X' || c == 'x' {
			digits++
		}
	}
	return digits == 10 || digits == 13
}

func looksLikeASIN(s string) bool {
	if len(s) != 10 {
		return false
	}
	if !(s[0] == 'B' && s[1] == '0') {
		return false
	}
	for i := 2; i < 10; i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}
