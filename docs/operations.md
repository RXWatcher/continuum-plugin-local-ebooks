# Operations — continuum-plugin-local-ebooks

## 1. Postgres schema setup

The plugin keeps its tables in a dedicated Postgres schema (namespace)
within a shared Continuum database. Create the role and schema before
installing the plugin:

```sql
CREATE ROLE plugin_ebooksdb LOGIN PASSWORD '<set-something-strong>';
CREATE SCHEMA ebooksdb AUTHORIZATION plugin_ebooksdb;
```

The plugin's DSN must set `search_path=ebooksdb` so its migrations
target that schema. Example DSN:

```
postgres://plugin_ebooksdb:<pwd>@db.internal:5432/continuum?search_path=ebooksdb&sslmode=disable
```

The migrations are idempotent — running them again against an already-
migrated schema is a no-op (tracked via `schema_migration` in that same
schema).

## 2. Library paths

Configure one or more absolute paths in the plugin admin UI as a JSON
array:

```json
["/srv/ebooks", "/mnt/extra"]
```

The plugin walks each path recursively for files with the supported
extensions: `.epub`, `.pdf`, `.mobi`, `.azw`, `.azw3`, `.fb2`. It only
reads — never writes. The library on disk is canonical.

## 3. Library scanning

Three trigger paths:

- **Admin button** — `POST /admin/scan` from the ebooks portal admin UI
  ("Scan now"). Returns immediately with the `scan_event_id`.
- **Scheduled** — every 6 hours via the SDK's `scheduled_task.v1` (cron
  `0 */6 * * *`). No config required.
- **Startup** — on first Configure after install, the plugin does not
  auto-scan. Run the admin trigger once to populate the library.

Files that disappear between scans are **soft-deleted** (marked
inactive); a subsequent scan that finds the file again will reactivate
the row. Hard deletion is manual.

Progress and history: `GET /admin/scan/status` returns the most recent
50 `scan_event` rows.

## 4. Metadata enrichment

The plugin enriches ebooks with external metadata via a Postgres-backed
queue. The `metadata_enrichment_worker` scheduled task drains the queue
once per minute (cron `* * * * *`).

### Cascade model

A single pass per ebook uses one source. Selection follows identifier
strength, in order:

1. **ISBN match** — if the parsed file carries an ISBN-10 or ISBN-13,
   the worker queries `metadata_scan_isbn_source` (default
   `openlibrary`) by that ISBN.
2. **ASIN match** — if no ISBN but an ASIN is present (common for MOBI /
   AZW / AZW3), the worker queries `metadata_scan_asin_source` (default
   `amazon`).
3. **Text fallback** — if neither identifier is present, the worker
   queries `metadata_scan_source` (default `openlibrary`) with the
   title and primary author string.

Per-pass behaviour is single-source: failure or low confidence does not
fall through to the next source within the same pass. To try a
different source, change the relevant config key and re-enqueue (see
backfill below).

### Enabled sources

Control which are active with `metadata_sources_enabled` (JSON array of
source IDs; defaults to all). Sources whose API keys are blank are
automatically disabled — the worker will skip them, and `Search` will
return no results from them.

### Triggering backfill

To re-enrich all ebooks (e.g. after adding an API key, changing region,
or switching `metadata_scan_source`):

```
POST /admin/metadata/backfill
```

This enqueues every ebook for re-enrichment. The worker drains the
queue at its next scheduled minute.

### Rate limiting and caching

- `metadata_rate_limit_rps` (default `5`) limits outbound requests per
  source.
- `metadata_cache_ttl_days` (default `30`) controls how long a positive
  result is cached before re-fetching. Negative results (404, no match)
  are cached with a shorter internal TTL.
- `metadata_default_region` (default `"us"`) sets the ISO country code
  for region-aware sources (notably Amazon, Goodreads, Douban).

### Cache eviction

Expired cache rows are evicted at the tail of each library scan, after
the walk completes and before the scan event is closed. There is no
separate eviction cron; if scans are not running, cache rows simply
remain until TTL is checked at read time.

### Inline enrichment

Set `scan_inline_enrich = true` to run enrichment synchronously after
each library scan finishes instead of queuing for the next worker run.
Suitable for small libraries; avoid on large ones because the scan
event will not close until every ebook has been processed.

## 5. Troubleshooting

### A scrape source is failing

The six HTML-scrape sources (Goodreads, Amazon, Anna's Archive,
Fantastic Fiction, ISFDB, LibraryThing, WorldCat, Douban) are
best-effort. Site markup changes break them and there is no SLA. The
aggregator logs the failure and continues with the remaining sources;
no operator action is needed unless a specific source is critical to
your library, in which case disable it via `metadata_sources_enabled`
to suppress log noise.

### Metadata is not appearing on an ebook

Check, in order:

1. The `metadata_enrichment_job` table — is there a row for the
   `ebook_id`? If not, scan didn't enqueue it (only ebooks lacking a
   prior successful enrichment are enqueued automatically).
2. The job's `status` and `last_error` columns — a permanent failure
   (e.g. 401 from a keyed source) parks the row.
3. `metadata_sources_enabled` — is the source you expect actually
   enabled?
4. API keys for `googlebooks` / `isbndb` / `hardcover` — empty key
   means the source is disabled.
5. The `metadata_cache` table — a cached negative result will suppress
   re-querying until `metadata_cache_ttl_days` elapses. Force a refresh
   by deleting the cache row and re-running backfill.

### `database_url` was rejected at Configure

The DSN must include `search_path=ebooksdb` and the role must own the
schema. Migrations also require `CREATE TABLE` on that schema; if you
restricted the role, grant it back temporarily for the first run.

## 6. Backups

The plugin's schema contains the catalog index, cover bytes, scan
history, and metadata cache. The on-disk ebook files are authoritative
content — losing the schema is recoverable via a rescan (file metadata
re-parses from each file). Cover bytes that came from sidecar files are
also recoverable; embedded covers re-extract from each file's container.

The metadata cache and enrichment queue **are not** recoverable; a fresh
schema after DR will re-fetch from external sources subject to rate
limits, which can take hours for a large library.

A periodic `pg_dump --schema=ebooksdb` is sufficient if you want to
avoid the rescan + re-enrichment cost after a DR event.
