-- Remove only what the up migration created: the built-in TMDB key (an admin's
-- own key has a different value and stays) and rows still empty.
DELETE FROM server_config
WHERE (key = 'api.tmdb_key' AND value = 'eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiI0OTdmMDk2NTBlOTlkNGJjZmM0NDE1NjNiYWVmY2NlYiIsIm5iZiI6MTU4NjUxMTg0NS40MTI5OTk5LCJzdWIiOiI1ZTkwM2ZlNWU2NDBkNjAwMWIxZjIzODMiLCJzY29wZXMiOlsiYXBpX3JlYWQiXSwidmVyc2lvbiI6MX0.nG3dwy0jKg2LKxyjipQScOocwA3pEyj2N8qXkI1V51Q')
   OR (key IN ('api.omdb_key', 'api.opensubtitles_key', 'api.tidb_key') AND value = '');
