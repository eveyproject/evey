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

# ── Privileged init phase (only when started as root via compose user: "0:0")──
# Perform the ONE privileged step — cgroup v2 delegation for per-plugin limits
# — then immediately drop to the unprivileged 'evey' user for EVERYTHING else
# (Meilisearch and the server). The server therefore never runs as root while
# it handles requests/plugins: a server compromise lands as UID 1000, not root.
# When the container starts as 'evey' (the image default), this block is skipped
# and behaviour is unchanged.
setup_cgroup_delegation() {
    CG=/sys/fs/cgroup
    [ -f "$CG/cgroup.controllers" ] || { echo "evey: cgroup v2 unavailable; per-plugin limits disabled" >&2; return 0; }
    _ctrls=$(cat "$CG/cgroup.controllers")
    case " $_ctrls " in *" memory "*) ;; *) echo "evey: cgroup memory controller not delegated; per-plugin limits disabled" >&2; return 0 ;; esac
    # evey-plugins is the delegated parent. The container's own processes go in
    # evey-plugins/server (a sibling leaf UNDER it) so the unprivileged server
    # can later migrate each plugin into evey-plugins/<id> WITHOUT crossing the
    # root cgroup — cgroup v2 only lets a delegatee move processes within the
    # subtree whose common ancestor it owns (here, evey-plugins).
    if ! mkdir -p "$CG/evey-plugins/server" 2>/dev/null; then
        echo "evey: /sys/fs/cgroup is read-only (add a rw mount); per-plugin limits disabled" >&2
        return 0
    fi
    # Move the container's processes into evey-plugins/server so root and
    # evey-plugins hold no processes directly and can delegate controllers
    # (cgroup v2 forbids enabling controllers on a cgroup that holds processes).
    if [ -f "$CG/cgroup.procs" ]; then
        while read -r _pid; do
            [ -n "$_pid" ] && { echo "$_pid" > "$CG/evey-plugins/server/cgroup.procs" 2>/dev/null || true; }
        done < "$CG/cgroup.procs"
    fi
    for _c in memory cpu pids io; do
        case " $_ctrls " in *" $_c "*) echo "+$_c" > "$CG/cgroup.subtree_control" 2>/dev/null || true ;; esac
    done
    for _c in memory cpu pids io; do
        case " $_ctrls " in *" $_c "*) echo "+$_c" > "$CG/evey-plugins/cgroup.subtree_control" 2>/dev/null || true ;; esac
    done
    # Hand the whole evey-plugins subtree to the server so it can create one
    # leaf per plugin AND migrate plugins within the subtree (needs write on
    # evey-plugins/cgroup.procs, the common ancestor).
    chown -R evey:evey "$CG/evey-plugins" 2>/dev/null || true
    echo "evey: cgroup delegation ready — per-plugin resource limits active"
}

if [ "$(id -u)" = "0" ]; then
    if [ "${EVEY_PLUGINS_CGROUP_DELEGATE:-}" = "true" ]; then
        setup_cgroup_delegation
    fi
    if ! command -v su-exec >/dev/null 2>&1; then
        echo "evey: su-exec missing — refusing to run the server as root" >&2
        exit 1
    fi
    # Re-exec this script as 'evey'; the re-exec'd run has id -u != 0 and skips
    # this block, continuing to Meilisearch + the server as UID 1000.
    exec su-exec evey:evey "$0" "$@"
fi
# ── From here we run as the unprivileged 'evey' user ─────────────────────────

EVEY_EMBEDDED_MEILI="${EVEY_EMBEDDED_MEILI:-true}"

if [ "$EVEY_EMBEDDED_MEILI" = "true" ]; then
    if [ -z "${MEILI_MASTER_KEY:-}" ]; then
        echo "evey: MEILI_MASTER_KEY required when EVEY_EMBEDDED_MEILI=true (set to a long random secret)" >&2
        exit 1
    fi

    MEILI_DATA_DIR="${MEILI_DATA_DIR:-/data/meili}"
    mkdir -p "$MEILI_DATA_DIR"

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
