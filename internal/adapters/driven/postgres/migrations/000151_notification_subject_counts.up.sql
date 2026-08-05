-- The notification LABEL was built in English at write time and stored whole
-- ("new_series:Lioness (1 season, 1 episode)"), so a French TV rendered
-- "Nouvelle série : Lioness (1 season, 1 episode)" - the clients translate the
-- prefix and copy the rest through untouched.
--
-- subject carries the bare name the label is about ("Lioness") and season_count
-- the seasons that came with it; episode_count already existed (000091). With
-- the three, a client formats the whole sentence in its own language.
--
-- Existing rows keep subject = '' on purpose: they were written before the
-- counts were data, and the clients fall back to parsing their stored title.
-- Backfilling would mean re-parsing an English string, which is the very thing
-- this replaces.
ALTER TABLE notifications
    ADD COLUMN IF NOT EXISTS subject      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS season_count INTEGER NOT NULL DEFAULT 0;
