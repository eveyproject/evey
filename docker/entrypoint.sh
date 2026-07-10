#!/bin/sh
# entrypoint for the evey image — supervises an embedded Meilisearch when
# requested, then execs the server. Designed to be PID 1 so signals flow
# through to both processes.
#
#   EVEY_EMBEDDED_MEILI=true   (default) → spawn meilisearch as a child,
#                                          wait for /health, then exec evey
#   EVEY_EMBEDDED_MEILI=false           → trust EVEY_MEILI_URL (must be set
#                                          to an external instance)
#
# When embedded, MEILI_MASTER_KEY is required so the index isn't reachable
# from anyone with /data write access (volume backups + co-tenant). The
# entrypoint exits 1 if it's missing in embedded mode.

set -eu

# ── PUID/PGID (linuxserver.io pattern) ────────────────────────────────────
# The container starts as root ONLY long enough to re-map the 'evey' user to
# the operator's PUID/PGID (compose env, defaults 1000:1000 = the historical
# behaviour), re-own Evey's INTERNAL state dirs, then re-exec this script as
# the unprivileged evey user — everything below the root block (Meili + the
# server) runs exactly as before. The point: file-writing features (seek-index
# repair, future remuxes) need the server to BE the owner of the user's media;
# with PUID the operator aligns the container on their files instead of
# chown-ing their library for us. Media mounts are NEVER touched here.
#
# Operators who refuse even transient root can keep `user: "UID:GID"` in
# compose instead: this block sees a non-root uid and no-ops, at the price of
# managing /data ownership themselves.
#
# Plugin isolation is unaffected: plugins are dropped to the separate
# evey-plugin uid (1001) by the capability-bearing launcher and are sandboxed
# by RBAC tokens + the resource monitor — none of that reads the server's
# numeric uid. PUID=1001 is refused so the two identities can never collide.
if [ "$(id -u)" = "0" ]; then
    # Zero-config default: when the operator sets NEITHER PUID nor PGID, adopt
    # the owner of the /media* mounts so the write features work out of the
    # box - running as the media owner is exactly what an operator would
    # configure by hand. Explicit PUID/PGID always wins; root-owned or
    # mixed-owner media falls back to 1000:1000 with a loud warning (we never
    # run as root and we never chown the operator's media).
    if [ -z "${PUID:-}" ] && [ -z "${PGID:-}" ]; then
        det_uid="" det_gid="" det_src="" det_mixed=""
        for m in /media*; do
            [ -d "$m" ] || continue
            u="$(stat -c %u "$m")"
            g="$(stat -c %g "$m")"
            if [ -z "$det_uid" ]; then
                det_uid="$u" det_gid="$g" det_src="$m"
            elif [ "$det_uid" != "$u" ]; then
                det_mixed="$det_mixed $m=$u:$g"
            fi
        done
        if [ -n "$det_mixed" ]; then
            echo "evey: media mounts have mixed owners ($det_src=$det_uid:$det_gid vs$det_mixed) - keeping uid 1000; unify ownership or set PUID/PGID explicitly, write features (seek-index repair) will fail on media the server does not own" >&2
        elif [ "$det_uid" = "0" ]; then
            echo "evey: media mounts are owned by root and the server never runs as root - keeping uid 1000; chown your media to a regular user (or set PUID/PGID), write features (seek-index repair) will fail until then" >&2
        elif [ "$det_uid" = "1001" ]; then
            echo "evey: media mounts are owned by uid 1001, reserved for the evey-plugin user - keeping uid 1000; chown your media or set PUID/PGID, write features (seek-index repair) will fail until then" >&2
        elif [ -n "$det_uid" ]; then
            PUID="$det_uid"
            PGID="$det_gid"
            echo "evey: media mounts owned by ${PUID}:${PGID} (detected on $det_src) - running the server as that user so write features (seek-index repair) work out of the box; set PUID/PGID to override"
        fi
    fi
    PUID="${PUID:-1000}"
    PGID="${PGID:-1000}"
    case "${PUID}${PGID}" in
        *[!0-9]*) echo "evey: PUID/PGID must be numeric (got PUID=$PUID PGID=$PGID)" >&2; exit 1 ;;
    esac
    if [ "$PUID" = "0" ] || [ "$PGID" = "0" ]; then
        echo "evey: running the server as root (PUID/PGID 0) is not supported" >&2
        exit 1
    fi
    if [ "$PUID" = "1001" ]; then
        echo "evey: PUID 1001 collides with the evey-plugin user (plugin UID separation would be void) — pick another UID" >&2
        exit 1
    fi
    [ "$(id -g evey)" = "$PGID" ] || groupmod -o -g "$PGID" evey
    [ "$(id -u evey)" = "$PUID" ] || usermod -o -u "$PUID" evey
    # Re-own Evey's internal state when the identity changed — /data, /cache
    # and the (rarely used) home. One-time cost after a PUID change; the
    # plugin dir owner is restored to evey-plugin exactly as the Dockerfile
    # set it (0750, plugins must not read the server's secret dirs).
    if [ "$(stat -c %u /data)" != "$PUID" ] || [ "$(stat -c %g /data)" != "$PGID" ]; then
        echo "evey: re-owning internal state to ${PUID}:${PGID} (one-time after a PUID/PGID change)"
        chown -R evey:evey /data /cache /home/evey
        chown evey-plugin:evey /data/plugins
    fi
    # Supplementary groups after the drop: evey's own /etc/group memberships
    # (what --init-groups would grant) PLUS the groups the container runtime
    # injected into this root process (compose `group_add:`, typically the
    # host's render/video group for /dev/dri). --init-groups alone would reset
    # to the group database and silently cut hardware transcode off from the
    # render nodes. Root's group 0 is never carried over.
    supp=""
    for g in $(id -G evey) $(id -G); do
        [ "$g" = "0" ] && continue
        case ",$supp," in *",$g,"*) ;; *) supp="${supp:+$supp,}$g" ;; esac
    done
    echo "evey: dropping privileges - server runs as uid=${PUID} gid=${PGID} groups=${supp}"
    exec setpriv --reuid evey --regid evey --groups "$supp" "$0" "$@"
fi

# From here on we are the unprivileged evey user (re-exec'd above, or the
# operator's `user:` override) — the server never runs as root. Per-plugin
# resource caps are enforced in userspace by the runner's resource monitor
# (it polls /proc and stops a plugin that exceeds its memory/PID/disk caps),
# so no cgroup delegation and no privileged init step is needed. A plugin
# compromise lands as the separate evey-plugin uid.

EVEY_EMBEDDED_MEILI="${EVEY_EMBEDDED_MEILI:-true}"

if [ "$EVEY_EMBEDDED_MEILI" = "true" ]; then
    if [ -z "${MEILI_MASTER_KEY:-}" ]; then
        echo "evey: MEILI_MASTER_KEY required when EVEY_EMBEDDED_MEILI=true (set to a long random secret)" >&2
        exit 1
    fi

    MEILI_DATA_DIR="${MEILI_DATA_DIR:-/data/meili}"
    mkdir -p "$MEILI_DATA_DIR"

    # ── Indexing memory budget (cgroup-aware) ──────────────────────────────
    # Meilisearch sizes its indexing arena from the MACHINE's RAM (defaults to
    # 2/3 of total), blind to the container limit — on a 2 GB NAS with a 1 GB
    # limit a library scan would aim for ~1.3 GB and OOM the container. Derive
    # the real budget (cgroup v2, then v1, then /proc/meminfo) and cap indexing
    # at ~20% of it, floor 128 MiB. Operators keep the last word: an explicit
    # MEILI_MAX_INDEXING_MEMORY in the environment is passed through untouched.
    if [ -z "${MEILI_MAX_INDEXING_MEMORY:-}" ]; then
        host_total=$(awk '/^MemTotal:/{print $2 * 1024}' /proc/meminfo)
        limit=""
        if [ -r /sys/fs/cgroup/memory.max ]; then
            limit=$(cat /sys/fs/cgroup/memory.max)
        elif [ -r /sys/fs/cgroup/memory/memory.limit_in_bytes ]; then
            limit=$(cat /sys/fs/cgroup/memory/memory.limit_in_bytes)
        fi
        case "$limit" in
            ''|max|*[!0-9]*) limit="$host_total" ;;
        esac
        # cgroup v1 reports "unlimited" as a huge sentinel; clamp to the host.
        [ "$limit" -gt "$host_total" ] && limit="$host_total"
        budget_mib=$((limit / 5 / 1048576))
        [ "$budget_mib" -lt 128 ] && budget_mib=128
        export MEILI_MAX_INDEXING_MEMORY="${budget_mib} MiB"
        echo "evey: memory budget $((limit / 1048576)) MiB detected — capping Meilisearch indexing memory at ${budget_mib} MiB"
    fi

    /usr/local/bin/meilisearch \
        --db-path "$MEILI_DATA_DIR" \
        --http-addr "127.0.0.1:7700" \
        --master-key "$MEILI_MASTER_KEY" \
        --env production \
        --no-analytics &
    MEILI_PID=$!

    # Propagate SIGTERM/SIGINT to Meili so `docker stop` returns promptly.
    # Once evey-server has been replaced by exec below, this trap is moot —
    # signals go straight to evey, and Meili dies with the container.
    trap 'kill -TERM "$MEILI_PID" 2>/dev/null || true; wait "$MEILI_PID" 2>/dev/null || true; exit 0' TERM INT

    # Wait up to 30s for Meili to answer /health. 0.5s × 60 polls.
    i=0
    until wget -q -O- "http://127.0.0.1:7700/health" >/dev/null 2>&1; do
        i=$((i + 1))
        if [ "$i" -ge 60 ]; then
            echo "evey: embedded Meilisearch did not become healthy in 30s" >&2
            kill -TERM "$MEILI_PID" 2>/dev/null || true
            exit 1
        fi
        # If Meili died early (e.g. corrupt index), fail loud.
        if ! kill -0 "$MEILI_PID" 2>/dev/null; then
            echo "evey: embedded Meilisearch exited before becoming healthy — check logs above" >&2
            exit 1
        fi
        sleep 0.5
    done

    # evey reads MEILI_MASTER_KEY directly (see internal/config/config.go)
    # so we only need to point the URL at the loopback instance — the key
    # is already in the env, passed from docker-compose.
    export EVEY_MEILI_URL="http://127.0.0.1:7700"
    echo "evey: embedded Meilisearch ready on 127.0.0.1:7700"
fi

# Hand off to evey-server. PID 1 becomes evey; if it exits, the container
# exits (and Docker restarts per restart policy). The embedded Meili is
# tied to the same PID namespace so it dies with us.
exec /evey
