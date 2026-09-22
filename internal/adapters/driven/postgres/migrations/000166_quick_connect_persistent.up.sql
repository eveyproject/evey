-- The pairing carries the kind of session the DEVICE asked for.
--
-- The lifetime used to be decided by the header on the call that COLLECTS the
-- tokens. A browser collects them over an event stream, and an EventSource
-- cannot set a header, so those pairings silently fell back to the short
-- session: a television paired that way asked for a password again a week
-- later. The device that starts the pairing can always say what it is, and
-- the pairing is still approved by a signed-in user.
ALTER TABLE quick_connect_requests
    ADD COLUMN IF NOT EXISTS persistent BOOLEAN NOT NULL DEFAULT FALSE;
