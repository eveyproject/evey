-- Admin-uploaded artwork was stored with the API enum name as its type
-- (ARTWORK_TYPE_PRIMARY / ARTWORK_TYPE_BACKDROP) instead of the domain one
-- (poster / backdrop), so no lookup ever found it. Give those rows the type
-- every reader expects.
UPDATE artwork SET type = 'poster' WHERE provider = 'custom' AND type = 'ARTWORK_TYPE_PRIMARY';
UPDATE artwork SET type = 'backdrop' WHERE provider = 'custom' AND type = 'ARTWORK_TYPE_BACKDROP';
