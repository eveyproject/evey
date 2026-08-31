-- bio_checked_at stamps the last time the metadata provider was asked for a
-- person's biography and answered WITHOUT one. Without it, a person the
-- provider genuinely has no bio for is re-fetched on every re-enrichment
-- pass forever, one round-trip per bio-less person per day. NULL means never
-- asked, or the provider has not yet answered.
ALTER TABLE people ADD COLUMN bio_checked_at TIMESTAMPTZ;
