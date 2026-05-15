ALTER TABLE library_path
  ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS media_type TEXT NOT NULL DEFAULT 'book';

UPDATE library_path
   SET name = path
 WHERE name = '';

ALTER TABLE library_path
  ALTER COLUMN name SET DEFAULT '';

