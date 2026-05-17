-- content_sig decouples change-detection from the primary key. Previously the
-- scanner derived ebook.id from (path,size,mtime) and UpsertEbook did
-- id=EXCLUDED.id on the (library_path_id,path) conflict, so editing a file
-- mutated the PK — which FK-violates against cover/metadata_enrichment_job
-- (ON DELETE CASCADE, no ON UPDATE CASCADE) and made every rescan of an
-- edited-and-covered book fail. The id is now stable per (library_path_id,
-- path); content_sig carries the (size,mtime) signature used to skip
-- unchanged files. Existing rows default to '' so the first post-migration
-- scan re-ingests each file once (one-time backfill, not an ongoing storm).
ALTER TABLE ebook
  ADD COLUMN IF NOT EXISTS content_sig TEXT NOT NULL DEFAULT '';
