CREATE TABLE media_item_translations (
    media_item_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    locale        TEXT NOT NULL,
    title         TEXT,
    overview      TEXT,
    tagline       TEXT,
    PRIMARY KEY (media_item_id, locale)
);

CREATE TABLE person_translations (
    person_id TEXT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    locale    TEXT NOT NULL,
    biography TEXT,
    PRIMARY KEY (person_id, locale)
);
