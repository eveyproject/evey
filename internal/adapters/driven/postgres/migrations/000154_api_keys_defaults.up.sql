-- Metadata provider keys as server_config rows from the first boot.
--
-- TMDB ships with a built-in key so a fresh install fetches metadata without
-- anyone creating a TMDB account first - the one step every media server
-- makes a newcomer do before seeing a single poster. It is the project's own
-- read-only token (Bearer, v4), shared by every install that does not bring
-- its own: an admin replaces it in Admin -> Server config -> API keys, or
-- seeds EVEY_TMDB_API_KEY, and the server prefers theirs (resolveAPIKey
-- treats this exact value as "the default", never as an admin's choice).
--
-- The other providers have no shared key to offer: their rows are created
-- empty so they exist in the panel from day one and an admin fills them in.
-- ON CONFLICT DO NOTHING: an install that already holds a key keeps it.
INSERT INTO server_config (key, value, updated_at) VALUES
    ('api.tmdb_key', 'eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiI0OTdmMDk2NTBlOTlkNGJjZmM0NDE1NjNiYWVmY2NlYiIsIm5iZiI6MTU4NjUxMTg0NS40MTI5OTk5LCJzdWIiOiI1ZTkwM2ZlNWU2NDBkNjAwMWIxZjIzODMiLCJzY29wZXMiOlsiYXBpX3JlYWQiXSwidmVyc2lvbiI6MX0.nG3dwy0jKg2LKxyjipQScOocwA3pEyj2N8qXkI1V51Q', NOW()),
    ('api.omdb_key', '', NOW()),
    ('api.opensubtitles_key', '', NOW()),
    ('api.tidb_key', '', NOW())
ON CONFLICT (key) DO NOTHING;
