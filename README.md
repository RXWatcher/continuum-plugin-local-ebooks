# continuum-plugin-ebooksdb

Local-filesystem ebook backend for Continuum. Scans a directory tree of
ebook files and exposes them to the ebooks portal via the
`ebook_backend.v1` advertised capability. Pure-Go format parsers; no
Calibre or external CLI dependencies at runtime.

## What it does

- Walks one or more configured library paths looking for ebook files
  (EPUB, PDF, MOBI, AZW, AZW3, FB2).
- Extracts title, authors, series, ISBN/ASIN, language, publication year,
  publisher, identifiers, and embedded cover art from each file. Files
  on disk are never modified.
- Serves catalog browsing, cover art (with thumb/medium resize), and
  ebook file delivery to the ebooks portal via gRPC + HTTP.
- Acts as a metadata aggregator over 15 external sources via the
  `metadata_provider.v1` capability.

## Metadata provider

The plugin advertises the `metadata_provider.v1` capability and bundles
fifteen sources. Three require API keys; the rest work out of the box
(several rely on HTML scraping and are best-effort).

| Source ID          | Service                        | API key                  |
|--------------------|--------------------------------|--------------------------|
| `openlibrary`      | Open Library                   | none                     |
| `googlebooks`      | Google Books                   | `googlebooks_api_key`    |
| `isbndb`           | ISBNdb                         | `isbndb_api_key`         |
| `hardcover`        | Hardcover (GraphQL)            | `hardcover_api_key`      |
| `goodreads`        | Goodreads (HTML scrape)        | none — best-effort       |
| `amazon`           | Amazon (HTML scrape)           | none — best-effort       |
| `annasarchive`     | Anna's Archive (HTML scrape)   | none                     |
| `gutenberg`        | Project Gutenberg via Gutendex | none                     |
| `bookbrainz`       | BookBrainz                     | none                     |
| `fantasticfiction` | Fantastic Fiction (scrape)     | none                     |
| `isfdb`            | ISFDB (scrape)                 | none                     |
| `librarything`     | LibraryThing (scrape)          | none                     |
| `internetarchive`  | Internet Archive               | none                     |
| `worldcat`         | WorldCat (scrape)              | none — best-effort       |
| `douban`           | Douban (Chinese-language)      | none — best-effort       |

**Trigger model**: gRPC `Search` queries all enabled sources in parallel
and aggregates results by confidence. Scan-time enrichment goes through
a Postgres-backed queue drained every minute by the
`metadata_enrichment_worker` scheduled task. Each pass uses a single
source per ebook, cascading by identifier strength: ISBN match first,
then ASIN, then a title/author text query as a last resort.

**Admin endpoint**: `POST /admin/metadata/backfill` enqueues all
unenriched ebooks for re-enrichment.

## What it does NOT do

- It does not modify your files. No tag rewrites, no moves, no renames.
- It does not accept book requests. A local backend cannot acquire new
  content — the ebooks portal handles request routing through other
  backends.
- It does not perform OCR or extract full-text content. Only metadata
  embedded in container headers is read.

## Format coverage

| Format | Metadata source                          | Cover                       |
|--------|------------------------------------------|-----------------------------|
| EPUB   | OPF package document (Dublin Core)       | manifest item or sidecar    |
| PDF    | PDF info dictionary + XMP                | first-page render is **not** done; cover from sidecar only |
| MOBI   | PalmDOC + EXTH header                    | EXTH record 201 or sidecar  |
| AZW    | Same as MOBI                             | EXTH record 201 or sidecar  |
| AZW3   | KF8 EXTH header                          | EXTH record 201 or sidecar  |
| FB2    | `<description><title-info>` XML          | inline base64 binary        |

Calibre series (`calibre:series` / `calibre:series_index`) is read from
EPUB OPF when present.

## Quick start

1. Provision Postgres role + schema (see `docs/operations.md`).
2. Configure `database_url` and `library_paths` in the plugin admin UI.
3. (Optional) Paste API keys for the three keyed sources:
   - `googlebooks_api_key` — Google Books API key
     ([console.cloud.google.com](https://console.cloud.google.com/)).
   - `isbndb_api_key` — ISBNdb subscription key
     ([isbndb.com](https://isbndb.com/)).
   - `hardcover_api_key` — Hardcover personal API token
     ([hardcover.app](https://hardcover.app/account/api)).
   Sources without keys are enabled automatically.
4. Click "Scan now" in the ebooks portal admin page, or wait for the
   6-hourly scheduled scan.

For full operator setup, see [`docs/operations.md`](docs/operations.md).

## Capabilities

| Capability                  | ID                          | Notes                                  |
|-----------------------------|-----------------------------|----------------------------------------|
| `ebook_backend.v1`          | `ebooksdb`                  | Catalog + file delivery                |
| `metadata_provider.v1`      | `ebooksdb_meta`             | Aggregator over 15 sources             |
| `http_routes.v1`            | `api`                       | `/api/v1/*` and `/admin/*`             |
| `scheduled_task.v1`         | `library_scan`              | Cron `0 */6 * * *`                     |
| `scheduled_task.v1`         | `metadata_enrichment_worker`| Cron `* * * * *`                       |

## Configuration

All config keys are set via the plugin admin UI. Required keys are
marked.

| Key | Required | Default | Description |
|-----|----------|---------|-------------|
| `database_url` | yes | — | Postgres DSN; must set `search_path=ebooksdb`. |
| `library_paths` | yes | — | JSON array of absolute paths to scan recursively. |
| `googlebooks_api_key` | no | — | Google Books API key. Source disabled if empty. |
| `isbndb_api_key` | no | — | ISBNdb subscription key. Source disabled if empty. |
| `hardcover_api_key` | no | — | Hardcover personal API token. Source disabled if empty. |
| `metadata_sources_enabled` | no | all sources | JSON array of source IDs to query. |
| `metadata_default_region` | no | `"us"` | ISO country code for region-aware sources. |
| `metadata_cache_ttl_days` | no | `30` | Days a positive cache entry is retained. |
| `metadata_rate_limit_rps` | no | `5` | Per-source request rate cap. |
| `scan_inline_enrich` | no | `false` | Run enrichment inline after scan completes. |
| `metadata_scan_source` | no | `"openlibrary"` | Default source used by the enrichment worker for text-query passes. |
| `metadata_scan_isbn_source` | no | `"openlibrary"` | Source used when the queued job carries an ISBN. |
| `metadata_scan_asin_source` | no | `"amazon"` | Source used when the queued job carries an ASIN but no ISBN. |

## HTTP routes

| Path         | Access          | Purpose                                    |
|--------------|-----------------|--------------------------------------------|
| `/api/v1/*`  | authenticated   | Catalog browsing, file delivery, covers    |
| `/admin/*`   | admin           | Scan triggers, enrichment backfill, status |
