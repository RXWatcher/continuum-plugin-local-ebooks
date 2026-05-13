CREATE TABLE library_path (
  id              BIGSERIAL    PRIMARY KEY,
  path            TEXT         NOT NULL UNIQUE,
  enabled         BOOLEAN      NOT NULL DEFAULT TRUE,
  last_scanned_at TIMESTAMPTZ,
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE ebook (
  id                TEXT         PRIMARY KEY,
  library_path_id   BIGINT       NOT NULL REFERENCES library_path(id) ON DELETE CASCADE,
  path              TEXT         NOT NULL,
  format            TEXT         NOT NULL,
  file_size         BIGINT       NOT NULL,
  mtime             TIMESTAMPTZ  NOT NULL,
  title             TEXT         NOT NULL DEFAULT '',
  author            TEXT         NOT NULL DEFAULT '',
  publisher         TEXT         NOT NULL DEFAULT '',
  year              TEXT         NOT NULL DEFAULT '',
  language          TEXT         NOT NULL DEFAULT '',
  genre             TEXT         NOT NULL DEFAULT '',
  isbn              TEXT         NOT NULL DEFAULT '',
  asin              TEXT         NOT NULL DEFAULT '',
  description       TEXT         NOT NULL DEFAULT '',
  page_count        INTEGER      NOT NULL DEFAULT 0,
  series            TEXT         NOT NULL DEFAULT '',
  series_pos        TEXT         NOT NULL DEFAULT '',
  deleted           BOOLEAN      NOT NULL DEFAULT FALSE,
  deleted_at        TIMESTAMPTZ,
  scanned_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ebook_path_idx ON ebook(library_path_id, path);
CREATE INDEX ebook_active_idx ON ebook(library_path_id, deleted) WHERE deleted = FALSE;

CREATE TABLE cover (
  ebook_id     TEXT    PRIMARY KEY REFERENCES ebook(id) ON DELETE CASCADE,
  content_type TEXT    NOT NULL,
  bytes        BYTEA   NOT NULL,
  source       TEXT    NOT NULL CHECK (source IN ('embedded','sidecar'))
);

CREATE TABLE scan_event (
  id              BIGSERIAL    PRIMARY KEY,
  library_path_id BIGINT       REFERENCES library_path(id) ON DELETE SET NULL,
  started_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
  finished_at     TIMESTAMPTZ,
  books_added     INTEGER      NOT NULL DEFAULT 0,
  books_changed   INTEGER      NOT NULL DEFAULT 0,
  books_deleted   INTEGER      NOT NULL DEFAULT 0,
  error_text      TEXT
);
CREATE INDEX scan_event_recent_idx ON scan_event(started_at DESC);

CREATE TABLE metadata_cache (
  cache_key     TEXT        PRIMARY KEY,
  source        TEXT        NOT NULL,
  region        TEXT        NOT NULL DEFAULT '',
  response_json JSONB       NOT NULL,
  not_found     BOOLEAN     NOT NULL DEFAULT FALSE,
  fetched_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX metadata_cache_source_fetched_idx
  ON metadata_cache(source, fetched_at);

CREATE TABLE metadata_enrichment_job (
  ebook_id     TEXT        PRIMARY KEY REFERENCES ebook(id) ON DELETE CASCADE,
  status       TEXT        NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending','completed','failed')),
  attempts     INTEGER     NOT NULL DEFAULT 0,
  run_after    TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_error   TEXT        NOT NULL DEFAULT '',
  enqueued_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at  TIMESTAMPTZ
);
CREATE INDEX metadata_enrichment_pending_idx
  ON metadata_enrichment_job(run_after)
  WHERE status = 'pending';
