# continuum-plugin-local-ebooks

Local-filesystem ebook backend for Continuum. Scans a directory tree of ebook files and exposes them to the ebooks portal via the `ebook_backend.v1` advertised capability. Pure-Go format parsers — no Calibre or external CLI dependencies at runtime.

The plugin also advertises the `metadata_provider.v1` capability and bundles fifteen external sources.

## What it does

- Walks one or more configured library paths looking for ebook files (EPUB, PDF, MOBI, AZW, AZW3, FB2).
- Extracts title, authors, series, ISBN/ASIN, language, publication year, publisher, identifiers, and embedded cover art from each file. Files on disk are **never modified**.
- Serves catalog browsing, cover art (with thumb/medium resize), and ebook file delivery to the ebooks portal via gRPC + HTTP.
- Acts as a metadata aggregator over 15 external sources via `metadata_provider.v1`.

## What it does NOT do

- It does not modify your files. No tag rewrites, no moves, no renames.
- It does not accept book requests. A local backend cannot acquire new content — the ebooks portal handles request routing through other backends.
- It does not perform OCR or extract full-text content. Only metadata embedded in container headers is read.

## Capabilities

| Capability | Notes |
|---|---|
| `ebook_backend.v1` | Catalog, search, cover art (with resize), ebook file delivery. |
| `metadata_provider.v1` | Aggregator over 15 sources (see below). |
| `scheduled_task.v1` (`library_scan`) | Periodic rescan of `library_paths`. |
| `scheduled_task.v1` (`metadata_enrichment_worker`) | Drains the enrichment queue (1m). |

## Format coverage

| Format | Metadata source | Cover |
|--------|------------------|-------|
| EPUB | OPF package document (Dublin Core) | manifest item or sidecar |
| PDF | PDF info dictionary + XMP | sidecar only (no first-page render) |
| MOBI | PalmDOC + EXTH header | EXTH record 201 or sidecar |
| AZW / AZW3 | EXTH header (MOBI / KF8) | EXTH record 201 or sidecar |
| FB2 | `<description><title-info>` XML | inline base64 binary |

Calibre series (`calibre:series` / `calibre:series_index`) is read from EPUB OPF when present.

## Metadata sources

Three require API keys; the rest work out of the box (several rely on HTML scraping and are best-effort).

| Source ID | Service | API key |
|-----------|---------|---------|
| `openlibrary` | Open Library | none |
| `googlebooks` | Google Books | `googlebooks_api_key` |
| `isbndb` | ISBNdb | `isbndb_api_key` |
| `hardcover` | Hardcover (GraphQL) | `hardcover_api_key` |
| `goodreads` | Goodreads (HTML scrape) | none — best-effort |
| `amazon` | Amazon (HTML scrape) | none — best-effort |
| `annasarchive` | Anna's Archive (HTML scrape) | none |
| `gutenberg` | Project Gutenberg via Gutendex | none |
| `bookbrainz` | BookBrainz | none |
| `fantasticfiction` | Fantastic Fiction (scrape) | none |
| `isfdb` | ISFDB (scrape) | none |
| `librarything` | LibraryThing (scrape) | none |
| `internetarchive` | Internet Archive | none |
| `worldcat` | WorldCat (scrape) | none — best-effort |
| `douban` | Douban (Chinese-language) | none — best-effort |

**Trigger model**: gRPC `Search` queries all enabled sources in parallel and aggregates results by confidence. Scan-time enrichment goes through a Postgres-backed queue drained every minute by the `metadata_enrichment_worker` scheduled task. Each pass uses a single source per ebook, cascading by identifier strength: ISBN → ASIN → title+author text query.

**Admin endpoint**: `POST /admin/metadata/backfill` enqueues all unenriched ebooks for re-enrichment.

## Configuration

| Key | Required | Description |
|---|---|---|
| `database_url` | yes | DSN for the `ebooksdb` schema. |
| `library_paths` | yes | JSON array of absolute filesystem paths. |
| `standalone_http_listen` | no | Bind a TCP listener for presigned-URL file delivery. Requires `stream_signing_secret`. |
| `stream_signing_secret` | conditional | 32-byte base64 HMAC, shared with the portal. Required when `standalone_http_listen` is set. |
| `metadata_sources_enabled` | no | JSON array of source IDs (default: all 15). |
| `metadata_default_region` | no | ISO country code (default `us`). |
| `metadata_cache_ttl_days` | no | Positive cache retention (default 30). |
| `metadata_rate_limit_rps` | no | Per-source RPS (default 5). |
| `scan_inline_enrich` | no | Run enrichment synchronously after a scan. |
| `metadata_scan_source` | no | Single source used by the enrichment worker. |
| `googlebooks_api_key` | conditional | Required to enable Google Books. |
| `isbndb_api_key` | conditional | Required to enable ISBNdb. |
| `hardcover_api_key` | conditional | Required to enable Hardcover. |

## Dependencies

- Postgres role + `ebooksdb` schema.
- Mounted filesystem with ebook files.
- Outbound HTTP access to enabled metadata sources.

## Install

```sql
CREATE ROLE plugin_ebooksdb LOGIN PASSWORD '<chosen>';
CREATE SCHEMA ebooksdb AUTHORIZATION plugin_ebooksdb;
```

Configure `database_url` and `library_paths` in the plugin admin UI. See [`docs/operations.md`](docs/operations.md) for the standalone-port + signed-URL setup.

## Build & test

```bash
make build
make test
```

## Status

v0.1.0 on branch `feat/scaffold`. Beta — many of the scraping sources are best-effort.
