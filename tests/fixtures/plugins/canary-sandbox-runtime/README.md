# Canary sandbox runtime probe

Not a malicious plugin — a benign canary that probes which host paths are
visible from inside the bwrap sandbox at runtime. Used to confirm the C2
mount whitelist actually blocks (vs just being declared in the bwrap argv).

## How to use

1. `zip -r canary.zip .` from this directory
2. Upload via `POST /api/v1/admin/plugins/upload` with form field `plugin`
3. Wait ~5s, then read the plugin log file:
   `docker exec evey-server cat /data/logs/plugins/<plugin-id>-<date>.log`

## Expected output (sandbox effective)

```
ROOT_LIST count=9 dirs=bin,data,dev,etc,lib,proc,sbin,tmp,usr
STAT ok /data type=dir size=60 kids=1 sample=plugins
STAT_BLOCKED /var ENOENT
STAT_BLOCKED /home ENOENT
STAT_BLOCKED /root ENOENT
STAT_BLOCKED /mnt ENOENT
STAT_BLOCKED /media ENOENT
STAT_BLOCKED /opt ENOENT
STAT_BLOCKED /srv ENOENT
STAT_BLOCKED /run ENOENT
STAT_BLOCKED /etc/passwd ENOENT
STAT_BLOCKED /etc/shadow ENOENT
STAT_BLOCKED /data/jwt.key ENOENT
STAT ok /proc/1/environ type=file size=0 read_ok len=274
CANARY done
```

The `/proc/1/environ` access is NOT a leak: thanks to `--unshare-pid`, PID
1 inside the sandbox is the canary itself, and reading its own environ is
not a sandbox bypass.

## Note on plugin state

The canary does NOT bind a Unix socket on `EVEY_PLUGIN_SOCK`, so the host
runtime times out and marks the plugin `PLUGIN_STATE_ERROR` after ~10s.
That's expected — the goal of this fixture is the file-probe output in
the log, not a long-lived plugin lifecycle.
