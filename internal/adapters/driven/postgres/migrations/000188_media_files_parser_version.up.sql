-- Parser-version-driven refile: a path parser that learns a layout (a season
-- folder above a file whose name carries no marker) changed nothing for rows
-- already scanned, as the TV scan resolves a known path to its row and keeps
-- the row's type and parent. parser_version records the library.ParserVersion
-- a row's path was last read at (0 = before versioning); the scan refiles
-- rows below the current version in place.
ALTER TABLE media_files
    ADD COLUMN IF NOT EXISTS parser_version INTEGER NOT NULL DEFAULT 0;
