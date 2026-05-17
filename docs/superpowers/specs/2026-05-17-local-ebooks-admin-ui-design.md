# Local Ebooks — Admin Operator Console (Design)

Date: 2026-05-17
Status: Approved (design); pending spec review before implementation planning.
Plugin: `continuum-plugin-local-ebooks`

## 1. Problem & Goal

`continuum-plugin-local-ebooks` scans local filesystem roots and exposes them
to the `continuum-ebooks` portal as an `ebook_backend.v1` source. Operators
need to run **multiple libraries, each with its own media type** (e.g.
`/srv/books` = `book`, `/srv/comics` = `comics`, `/srv/manga` = `manga`).

The backend already models this end to end (`library_path` rows carry
`media_type`; the catalog JOINs it; per-`library_id` filtering and
per-media-type stats work; the portal maps presentation shelves to backend
`library_id`). The gaps:

1. Libraries can only be configured by hand-editing the host global-config
   `library_paths` JSON blob; there is **no management UI**.
2. `Configure` COALESCE-updates `library_path` on every call and never
   removes de-configured libraries — a destructive, split-brain-prone sync.

Goal: a **full operator console** (web UI) for this plugin that owns library
configuration, plus the backend changes to make the plugin DB the
authoritative, non-destructively-seeded source of truth.

## 2. Scope

In scope:
- Web admin console: Libraries (CRUD + per-library scan), Scans, Metadata
  (queue + backfill), Diagnostics.
- Admin REST API for library CRUD + per-library scan.
- Make plugin DB (`library_path`) the source of truth; `Configure` becomes a
  non-destructive seed.
- Transactional library delete with catalog cascade.

Out of scope (explicitly):
- Portal-side presentation libraries (`portal_library` in
  `continuum-ebooks`) — a separate layer the portal admin owns.
- The M1 ebook-PK-churn fix (separate tracked follow-up; needs its own
  migration; not coupled to this work).
- Authn/z in the SPA — the host enforces admin access on `/admin/*`.

## 3. Architecture & Serving

- New `web/` SPA in the repo, mirroring `continuum-ebooks` exactly:
  React 19 + Vite 5 + TypeScript + Tailwind 4 + shadcn/ui +
  `@tanstack/react-query` + `react-router`; layout `web/src/{pages,components,lib}`;
  `web/embed.go` with `//go:embed all:dist` exposing `FSEmbed()` /
  `http.FileSystem`.
- Served by the plugin's existing stdlib `http.ServeMux` (reached via
  `httproutes.Server` over the gRPC `HttpRoutes` service and the optional
  standalone listener). Add static asset serving + an SPA fallback
  (`NotFound` → `index.html`, including the `/admin/assets/` → `/assets/`
  rewrite), reusing the ebooks portal's fallback logic verbatim.
- Console mounts under `/admin/*`, already declared `access: "admin"` in the
  manifest, so the **host enforces admin auth**; the SPA does no auth.
- Manifest: add an `assets` route for the bundle; mark the `admin` route
  `navigable: true`, `navigation_label: "Local Ebooks"`,
  `navigation_kind: "admin"` so it appears in the host admin nav (identical
  to the ebooks portal registration).
- `?token=` is captured into memory on first load for API calls (copied from
  ebooks `lib/api.ts`).

Trust/ownership boundary: SPA ↔ admin REST API by JSON only; the admin API
is the sole mutator of `library_path`; `Configure` and the scanner are
readers/seeders of that table; no shared state beyond the JSON API.

## 4. Source of Truth

The `library_path` table in the plugin DB is authoritative.

- Admin API does full CRUD against it.
- `Configure` library sync is rewritten from a COALESCE upsert to
  `INSERT … ON CONFLICT (path) DO NOTHING` — a true non-destructive,
  idempotent **seed**: a host-configured path that does not yet exist is
  created (so declarative first-deploy still works); existing rows are never
  updated or deleted by `Configure`.
- Library removal is an explicit UI/API action only — never inferred from a
  host-config diff. This deletes the entire "de-config cleanup" failure mode
  rather than implementing it.

Accepted trade-off: the exact library set is no longer reproducible from
host config alone (the UI can mutate it). The plugin is already fully
stateful (catalog/covers/scan/metadata all live in its Postgres DB and must
be backed up), so library config riding in that DB adds no new backup
burden. A future explicit export/import is the clean mitigation if
config-only reproducibility is ever required.

## 5. Data Model

Reuse `library_path` (`id`, `path` UNIQUE, `name`, `media_type`, `enabled`,
`last_scanned_at`). Add `created_at` / `updated_at` if absent (small
forward-only migration; idempotent `ADD COLUMN IF NOT EXISTS`).

New store methods (each a single, independently testable unit):
- `CreateLibrary(ctx, {path,name,media_type,enabled}) (id, error)`
- `UpdateLibrary(ctx, id, {name,media_type,enabled}) error` (path immutable)
- `DeleteLibrary(ctx, id) error` — one transaction: delete dependent
  `metadata_enrichment_job` and `cover` rows for the library's ebooks,
  delete `ebook` rows for `library_path_id = id`, delete the `library_path`
  row; commit.
- Existing list/scan/seed methods retained; the seed path switches to
  insert-if-absent.

## 6. Admin REST API

All under `/admin`, host-enforced admin auth, JSON envelope consistent with
the existing `writeJSON`/`writeError`.

| Method | Path | Purpose |
|---|---|---|
| GET | `/admin/libraries` | list libraries (+ book counts, last scanned) |
| POST | `/admin/libraries` | create `{path,name,media_type,enabled}` |
| PATCH | `/admin/libraries/{id}` | update `name`/`media_type`/`enabled` |
| DELETE | `/admin/libraries/{id}` | delete library (cascade, transactional) |
| POST | `/admin/libraries/{id}/scan` | scan one library |
| POST | `/admin/scan` | scan all (exists) |
| GET | `/admin/scans` | scan history (exists) |
| GET | `/admin/metadata/queue` | enrichment queue stats |
| POST | `/admin/metadata/backfill` | bulk enqueue (exists) |
| GET | `/admin/diagnostics` | health/overview (exists) |

### Decisions

1. **`path` is immutable.** Changing a root = delete + re-add. Avoids
   orphan/rescan churn and ID confusion. `name`/`media_type`/`enabled` are
   freely mutable. Changing `media_type` is instant (JOIN-resolved at query
   time — no rescan).
2. **Delete = hard cascade in one transaction** (see `DeleteLibrary`).
   Rationale: the catalog rows are fully derived from on-disk files;
   re-adding the library rescans them. Cleaner than soft-delete tombstones
   for a deliberate admin removal. Distinct from the scanner's per-file
   `SoftDelete` (missing-file reconciliation), which is unchanged.
3. **`Configure` seed = `INSERT … ON CONFLICT (path) DO NOTHING`** (see §4).
4. **Per-library scan reuses existing scan logic.** `scanner.Walk` is
   already per-`library_path`; `runScan` is refactored so the scan_event
   audit/finish path is shared between "scan one" (the new endpoint, given
   a `library_path` id) and "scan all" (the existing loop). No new scan
   logic — only a scoped entrypoint over the same `Walk` + audit code.

### Validation & errors

- `media_type` ∈ {`book`,`comics`,`manga`,`documents`} else `400`.
- `path` must be absolute, `filepath.Clean`-ed, and an existing directory
  at create time, else `400` (fail fast rather than a silently empty
  library).
- Duplicate `path` → `409` (DB UNIQUE).
- Unknown `id` → `404`.
- Validation logic lives in one small pure package shared by the admin API
  and the `Configure` seed.

## 7. SPA

shadcn `Tabs` shell (mirroring the ebooks Admin shell):

1. **Libraries** (core): table — path · name · media type · enabled · last
   scanned · book count. Add via dialog (path, name, `media_type` select,
   enabled). Edit name/media_type/enabled. Delete behind a confirm dialog
   stating the cascade ("removes N books from the catalog; files on disk are
   untouched"). Per-row "Scan now".
2. **Scans**: `scan_event` history (started/finished, added/changed/
   deleted/failed, error text) + "Scan all".
3. **Metadata**: enrichment queue stats + "Backfill all".
4. **Diagnostics**: read-only health cards — DB reachable, catalog totals,
   by-media-type breakdown, config snapshot.

`lib/api.ts`: copied token capture + typed fetch wrappers; `react-query`
for fetch/cache; mutations invalidate the relevant query keys.

### Error handling (applies the bug classes fixed in the portal audit)

- API client **throws on non-2xx with the server message** — never a silent
  empty list; query errors render an error banner, not a misleading empty
  state.
- Forms are modal/explicit, so there is no background-refetch "draft wipe"
  class to design around.
- Destructive delete gated by the confirm dialog; create form surfaces
  `400`/`409` inline; success/error toasts (`sonner`).

## 8. Testing

- Go: pure validation helpers (`media_type` enum, path) — TDD unit tests.
  Admin handler tests with a fake store (the established `ebookbackend`
  test pattern) covering CRUD happy/error paths, **delete cascade**, and
  **`Configure` insert-if-absent seed**. DB-backed `Create/Update/Delete`
  tests behind the existing Postgres-skip harness.
- Frontend: `tsc -b && vite build` is the compile gate (consistent with the
  ebooks portal; no new JS test harness is introduced).
- Cross-plugin end-to-end (portal consuming the libraries) is verified
  manually; not automated here.

## 9. Build & Repo Impact

Adds a Node/pnpm/Vite pipeline, an embedded `dist`, and a Makefile/CI build
target to a currently pure-Go repo — copied from the ebooks portal setup.
`web/dist` is git-ignored; the embed builds from a produced `dist`.

## 10. Risks

- New frontend toolchain in a pure-Go repo (mitigated: copied, proven setup
  from the sibling plugin).
- Library config no longer reproducible from host config alone (accepted;
  see §4).
- `DeleteLibrary` cascade is irreversible by design — mitigated by the
  explicit confirm dialog and the fact that on-disk files are untouched
  (re-add rescans).

## 11. Out-of-scope follow-ups (tracked separately)

- M1: ebook-id PK churn on file edits (needs a `content_sig` migration).
- Optional library config export/import for GitOps reproducibility.
