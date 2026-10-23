DROP TABLE IF EXISTS plugin_trusted_publishers;
ALTER TABLE plugins DROP COLUMN IF EXISTS publisher_key_pem;
ALTER TABLE plugins DROP COLUMN IF EXISTS publisher_key_fingerprint;
