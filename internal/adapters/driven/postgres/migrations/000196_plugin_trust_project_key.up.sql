-- The Evey project signs its own plugin releases (the evey-plugin-* and
-- evey-theme-* repositories) with this key. Trusting it here lets them install
-- under the enforce signature policy with no setup. It is not a seeded row:
-- the boot prune keeps only the configured operator key, and an admin who
-- does not want it can remove it from the panel. A key already trusted under
-- another name is left as the admin named it.
INSERT INTO plugin_trusted_publishers (fingerprint, name, public_key_pem, seeded)
VALUES (
    'sha256:c29eb603b19d99a50f3fa6c94b7e92c41710779261e73e0fa31d53a78902e09a',
    'Evey project',
    E'-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEUnXNPTa3ZFH3ltB3Yw8Pv3gSmW2G\noeaSoMgn7IfZ3Df8q6/7jngtYZ8CrFLHCbbAgDq/bFAtUMl2TqSlfPkmfA==\n-----END PUBLIC KEY-----\n',
    FALSE
)
ON CONFLICT (fingerprint) DO NOTHING;
