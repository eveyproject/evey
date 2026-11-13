DELETE FROM plugin_trusted_publishers
WHERE fingerprint = 'sha256:c29eb603b19d99a50f3fa6c94b7e92c41710779261e73e0fa31d53a78902e09a'
  AND name = 'Evey project'
  AND seeded = FALSE;
