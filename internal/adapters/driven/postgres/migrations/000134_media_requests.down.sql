DROP TABLE IF EXISTS media_request_access;
DROP TABLE IF EXISTS media_requests;
DELETE FROM server_config WHERE key = 'mediarequest.enabled_default';
