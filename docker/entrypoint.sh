#!/bin/sh
# entrypoint for the evey image - supervises the embedded services (Postgres,
# Meilisearch) when requested, then runs the server. Designed to be PID 1 so
# signals flow through to every process.
#
#   EVEY_EMBEDDED_POSTGRES  unset (default) → embedded when EVEY_DB_HOST is
#                                             unset, external otherwise
#                           true / false    → explicit
#   EVEY_EMBEDDED_MEILI=true   (default)    → spawn meilisearch as a child,
#                                             wait for /health, then run evey
#   EVEY_EMBEDDED_MEILI=false               → trust EVEY_MEILI_URL (must be set
#                                             to an external instance)
#
# Secrets the image owns are generated on first boot and persisted under
# /data/keys (0600): the embedded Postgres password and the embedded
# Meilisearch master key. An explicit EVEY_DB_PASSWORD / MEILI_MASTER_KEY in
# the environment always wins, so an operator bringing their own database or
# search cluster configures it exactly as before. The server reads the files
# through EVEY_DB_PASSWORD_FILE / MEILI_MASTER_KEY_FILE (baked into the image),
# which is what makes `docker exec evey-server evey admin ...` work without
# the entrypoint's environment.

set -eu

# ── Subcommands that need no service ─────────────────────────────────────
# `docker run --rm gravityz/evey version` identifies the image without
# Postgres, Meili or the privilege drop; help, man and completion need nothing
# either. Every other invocation (the server, `admin ...`) goes through the
# full boot below so an embedded database is up when the command runs.
case "${1:-}" in
    version|--version|-v|help|--help|-h|man|completion)
        exec /evey "$@" ;;
esac

# ── PUID/PGID (linuxserver.io pattern) ────────────────────────────────────
# The container starts as root ONLY long enough to re-map the 'evey' user to
# the operator's PUID/PGID (compose env, defaults 1000:1000 = the historical
# behaviour), re-own Evey's INTERNAL state dirs, then re-exec this script as
# the unprivileged evey user - everything below the root block (Meili + the
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
# by RBAC tokens + the resource monitor - none of that reads the server's
# numeric uid. PUID=1001 is refused so the two identities can never collide.
if [ "$(id -u)" = "0" ]; then
    # Whether the plugin subsystem will actually run. Computed HERE, before the
    # media-owner detection below, because uid 1001 is only reserved when
    # plugins run: the auto-detection and the explicit PUID path must apply the
    # SAME policy, or the same NAS is adopted when the operator writes
    # PUID=1001 and refused when they let us detect it.
    # Matches strconv.ParseBool's false set, like the Go side reading this var.
    case "${EVEY_PLUGINS_ENABLED:-true}" in
        0|f|F|false|FALSE|False) evey_plugins_on=0 ;;
        *) evey_plugins_on=1 ;;
    esac

    # Zero-config default: when the operator sets NEITHER PUID nor PGID, adopt
    # the owner of the /media* mounts so the write features work out of the
    # box - running as the media owner is exactly what an operator would
    # configure by hand. Explicit PUID/PGID always wins; we never run as root
    # and we never chown the operator's media. With mixed owners across
    # mounts, adopt the uid owning the MOST mounts and join the other mounts'
    # groups as supplementary groups (best effort: those need group-writable
    # files for repairs). Only root/1001-owned everything falls back to
    # 1000:1000 with a loud warning.
    media_extra_gids=""
    if [ -z "${PUID:-}" ] && [ -z "${PGID:-}" ]; then
        det_uid="" det_gid="" det_src="" det_count=0 det_seen=""
        for m in /media*; do
            [ -d "$m" ] || continue
            det_seen="$det_seen $m=$(stat -c '%u:%g' "$m")"
            u="$(stat -c %u "$m")"
            # root cannot be adopted (we never run as root); 1001 is the
            # evey-plugin uid and is only reserved while plugins actually run.
            if [ "$u" = "0" ]; then continue; fi
            if [ "$u" = "1001" ] && [ "$evey_plugins_on" = "1" ]; then continue; fi
            count=0
            for n in /media*; do
                [ -d "$n" ] || continue
                [ "$(stat -c %u "$n")" = "$u" ] && count=$((count + 1))
            done
            if [ "$count" -gt "$det_count" ]; then
                det_uid="$u" det_gid="$(stat -c %g "$m")" det_src="$m" det_count="$count"
            fi
        done
        if [ -n "$det_uid" ]; then
            PUID="$det_uid"
            PGID="$det_gid"
            leftover=""
            for m in /media*; do
                [ -d "$m" ] || continue
                [ "$(stat -c %u "$m")" = "$det_uid" ] && continue
                leftover="$leftover $m=$(stat -c '%u:%g' "$m")"
                g="$(stat -c %g "$m")"
                if [ "$g" = "0" ] || [ "$g" = "$PGID" ]; then continue; fi
                case " $media_extra_gids " in
                    *" $g "*) ;;
                    *) media_extra_gids="$media_extra_gids $g" ;;
                esac
            done
            if [ -n "$leftover" ]; then
                join_msg=""
                [ -n "$media_extra_gids" ] && join_msg=" and joining the other mounts' groups (${media_extra_gids# })"
                echo "evey: media mounts have several owners - running as ${PUID}:${PGID} (owns $det_count, detected on $det_src)$join_msg; repairs on$leftover need group-writable files - unify ownership or set PUID/PGID to silence this" >&2
            else
                echo "evey: media mounts owned by ${PUID}:${PGID} (detected on $det_src) - running the server as that user so write features (seek-index repair) work out of the box; set PUID/PGID to override"
            fi
        elif [ -n "$det_seen" ]; then
            if [ "$evey_plugins_on" = "1" ]; then
                echo "evey: no adoptable media owner (root, and uid 1001 while plugins are on:$det_seen) - keeping uid 1000; chown your media to a regular user, set PUID/PGID, or set EVEY_PLUGINS_ENABLED=false if 1001 is your own uid; write features (seek-index repair) will fail until then" >&2
            else
                echo "evey: no adoptable media owner (root is reserved:$det_seen) - keeping uid 1000; chown your media to a regular user or set PUID/PGID, write features (seek-index repair) will fail until then" >&2
            fi
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
    if [ "$PUID" = "1001" ] && [ "$evey_plugins_on" = "1" ]; then
        echo "evey: PUID 1001 collides with the evey-plugin user (plugin UID separation would be void) - pick another UID, or set EVEY_PLUGINS_ENABLED=false" >&2
        exit 1
    fi
    if [ "$PUID" = "1001" ]; then
        echo "evey: PUID 1001 is the evey-plugin user; allowed because EVEY_PLUGINS_ENABLED is false - do not re-enable plugins under this UID" >&2
    fi
    [ "$(id -g evey)" = "$PGID" ] || groupmod -o -g "$PGID" evey
    [ "$(id -u evey)" = "$PUID" ] || usermod -o -u "$PUID" evey
    # Re-own Evey's internal state when the identity changed - /data, /cache
    # and the (rarely used) home, EACH on its own test. One-time cost after a
    # PUID change. Keying everything on /data alone left /cache at the image's
    # 1000:1000 whenever the operator had pre-created ./evey-data as PUID
    # (Synology's Container Manager insists on existing folders): the named
    # cache volume was then unwritable and the server crash-looped. The
    # SERVER owns /data/plugins (it extracts installs there; giving it to
    # evey-plugin broke every install with EACCES in prod 2026-07-11, and
    # would let a plugin tamper with its own validated files); mode 750 keeps
    # it group-readable for the dropped plugin uid and closed to others.
    for d in /data /cache /home/evey; do
        [ -d "$d" ] || continue
        if [ "$(stat -c %u "$d")" != "$PUID" ] || [ "$(stat -c %g "$d")" != "$PGID" ]; then
            echo "evey: re-owning $d to ${PUID}:${PGID} (one-time after a PUID/PGID change)"
            chown -R evey:evey "$d"
        fi
    done
    mkdir -p /data/plugins
    chown evey:evey /data/plugins
    chmod 750 /data/plugins
    # Supplementary groups after the drop: evey's own /etc/group memberships
    # (what --init-groups would grant) PLUS the groups the container runtime
    # injected into this root process (compose `group_add:`, typically the
    # host's render/video group for /dev/dri) PLUS the groups of media mounts
    # the adopted uid does not own (mixed-owner best effort above).
    # --init-groups alone would reset to the group database and silently cut
    # hardware transcode off from the render nodes. Root's group 0 is never
    # carried over.
    supp=""
    for g in $(id -G evey) $(id -G) $media_extra_gids; do
        [ "$g" = "0" ] && continue
        case ",$supp," in *",$g,"*) ;; *) supp="${supp:+$supp,}$g" ;; esac
    done
    echo "evey: dropping privileges - server runs as uid=${PUID} gid=${PGID} groups=${supp}"
    exec setpriv --reuid evey --regid evey --groups "$supp" "$0" "$@"
fi

# From here on we are the unprivileged evey user (re-exec'd above, or the
# operator's `user:` override) - the server never runs as root. Per-plugin
# resource caps are enforced in userspace by the runner's resource monitor
# (it polls /proc and stops a plugin that exceeds its memory/PID/disk caps),
# so no cgroup delegation and no privileged init step is needed. A plugin runs
# as the separate evey-plugin uid only in the basic sandbox; under Landlock (what
# auto usually resolves to) it keeps THIS uid, confined by Landlock + seccomp.

# ── Generated secrets ────────────────────────────────────────────────────
# ensure_secret <file> <label>: print the secret stored at <file>, creating a
# 64-hex-char one (0600, atomic rename) when the file is absent or empty. The
# keys dir is owner-only: with plugins dropped to uid 1001, plain DAC keeps
# these unreadable to them even without Landlock (same posture as jwt.key).
KEYS_DIR=/data/keys
ensure_secret() {
    if [ ! -s "$1" ]; then
        mkdir -p "$KEYS_DIR"
        chmod 700 "$KEYS_DIR"
        (umask 077; od -An -N32 -tx1 /dev/urandom | tr -d ' \n' > "$1.tmp" && mv "$1.tmp" "$1")
        echo "evey: generated the $2 at $1 - keep /data/keys with the rest of /data" >&2
    fi
    cat "$1"
}

# ── Embedded Postgres ────────────────────────────────────────────────────
# Default = embedded when the operator names no database host, so the
# quick-start is ONE container with no secret to invent; every existing
# compose (which sets EVEY_DB_HOST=postgres) keeps its external database
# untouched. The cluster lives in /data/postgres, next to the JWT key and the
# search index, so "back up ./evey-data" stays the whole story. It listens on
# loopback only and asks a password over TCP (scram): any uid in the container
# can reach 127.0.0.1:5432 - a plugin included - but only the server holds
# the password. The unix socket (owner-only dir) asks it too: a Landlock plugin
# shares this uid and the role is the cluster superuser, while /data/keys stays
# outside its allowlist. Boot-time admin passes it through PGPASSWORD.
PG_BIN=/usr/lib/postgresql/17/bin
PG_MAJOR=17
PGDATA_DIR="${EVEY_POSTGRES_DATA_DIR:-/data/postgres}"
PG_SOCKET_DIR=/tmp/evey-pg
case "${EVEY_EMBEDDED_POSTGRES:-}" in
    true|false) ;;
    '')
        if [ -z "${EVEY_DB_HOST:-}" ]; then EVEY_EMBEDDED_POSTGRES=true; else EVEY_EMBEDDED_POSTGRES=false; fi ;;
    *)
        echo "evey: EVEY_EMBEDDED_POSTGRES must be true or false (got '$EVEY_EMBEDDED_POSTGRES')" >&2
        exit 1 ;;
esac

PG_PID=""
pg_fresh=0
if [ "$EVEY_EMBEDDED_POSTGRES" = "true" ]; then
    case "${EVEY_DB_HOST:-127.0.0.1}" in
        127.0.0.1|localhost|::1) ;;
        *)
            echo "evey: EVEY_EMBEDDED_POSTGRES=true but EVEY_DB_HOST='$EVEY_DB_HOST' points elsewhere - unset one of them (an external database wants EVEY_EMBEDDED_POSTGRES=false)" >&2
            exit 1 ;;
    esac
    if [ ! -x "$PG_BIN/postgres" ]; then
        echo "evey: this image ships no embedded Postgres ($PG_BIN/postgres missing) - set EVEY_DB_HOST to an external database" >&2
        exit 1
    fi
    pg_user="${EVEY_DB_USER:-evey}"
    pg_db="${EVEY_DB_NAME:-evey}"
    pg_port="${EVEY_DB_PORT:-5432}"
    pg_pass_file="${EVEY_DB_PASSWORD_FILE:-$KEYS_DIR/postgres.password}"
    if [ -n "${EVEY_DB_PASSWORD:-}" ]; then
        pg_pass="$EVEY_DB_PASSWORD"
    else
        pg_pass="$(ensure_secret "$pg_pass_file" "embedded Postgres password")"
        export EVEY_DB_PASSWORD_FILE="$pg_pass_file"
    fi

    # A cluster written by another major version cannot be opened by this
    # binary: refuse loudly with the way out, never let Postgres fail on an
    # opaque catalogue error after a routine `compose pull`.
    if [ -s "$PGDATA_DIR/PG_VERSION" ]; then
        have="$(cat "$PGDATA_DIR/PG_VERSION")"
        if [ "$have" != "$PG_MAJOR" ]; then
            echo "evey: $PGDATA_DIR holds a Postgres $have cluster but this image ships Postgres $PG_MAJOR - dump it with the previous image, move the directory aside, then restore (docs/operations/advanced-deployments.md, 'Postgres major upgrade')" >&2
            exit 1
        fi
    elif [ -d "$PGDATA_DIR" ] && [ -n "$(ls -A "$PGDATA_DIR" 2>/dev/null)" ]; then
        echo "evey: $PGDATA_DIR is not empty and is not a Postgres cluster - move it aside or point EVEY_POSTGRES_DATA_DIR elsewhere" >&2
        exit 1
    else
        echo "evey: initialising the embedded Postgres $PG_MAJOR cluster in $PGDATA_DIR"
        pg_fresh=1
        mkdir -p "$PGDATA_DIR"
        chmod 700 "$PGDATA_DIR"
        # initdb needs the password in a file, never on the command line.
        pwtmp="$(mktemp)"
        chmod 600 "$pwtmp"
        printf '%s\n' "$pg_pass" > "$pwtmp"
        "$PG_BIN/initdb" -D "$PGDATA_DIR" -U "$pg_user" --pwfile="$pwtmp" \
            --auth-local=scram-sha-256 --auth-host=scram-sha-256 \
            --encoding=UTF8 --locale=C.UTF-8 >/dev/null
        rm -f "$pwtmp"
    fi

    # Clusters initialised before the socket asked a password carry initdb's
    # `local ... all trust` lines: only those exact lines move to scram (the role
    # password was set by --pwfile and realigned every boot). Anything else is
    # an operator edit, left alone but reported.
    pg_hba="$PGDATA_DIR/pg_hba.conf"
    pg_hba_gen='^(local[[:space:]]+(all|replication)[[:space:]]+all[[:space:]]+)trust([[:space:]]*)$'
    if grep -Eq "$pg_hba_gen" "$pg_hba"; then
        (umask 077; sed -E "s/$pg_hba_gen/\\1scram-sha-256\\3/" "$pg_hba" > "$pg_hba.evey-tmp")
        mv "$pg_hba.evey-tmp" "$pg_hba"
        echo "evey: embedded Postgres unix socket now asks the password (pg_hba.conf: initdb's local trust lines moved to scram-sha-256)"
    fi
    if grep -Eq '^[[:space:]]*local[[:space:]].*[[:space:]](trust|peer|ident)([[:space:]]|$)' "$pg_hba"; then
        echo "evey: WARNING $pg_hba has customised local lines that skip the password (trust/peer/ident) - left untouched, but a plugin sharing the server uid can then reach the database as superuser; switch them to scram-sha-256" >&2
    fi

    mkdir -p "$PG_SOCKET_DIR"
    chmod 700 "$PG_SOCKET_DIR"

    # pg_start <listen_addresses> [-c ...]: run Postgres in the background and
    # wait until it accepts connections. pg_isready never authenticates (an auth
    # request already counts as ready), so it needs no password.
    pg_start() {
        pg_listen="$1"; shift
        "$PG_BIN/postgres" -D "$PGDATA_DIR" \
            -c listen_addresses="$pg_listen" -c port="$pg_port" \
            -c unix_socket_directories="$PG_SOCKET_DIR" \
            -c log_min_messages=warning -c log_line_prefix='postgres: %m [%p] ' "$@" &
        PG_PID=$!
        # Up to 120s: a crash-recovery replay after an unclean stop can take a
        # while on a slow disk, and a not-yet-ready database is not a failure.
        i=0
        # -d postgres: pg_isready opens a session named after the user by default,
        # which logs a spurious FATAL before the evey database exists.
        until "$PG_BIN/pg_isready" -q -h "$PG_SOCKET_DIR" -p "$pg_port" -d postgres 2>/dev/null; do
            i=$((i + 1))
            if [ "$i" -ge 240 ]; then
                echo "evey: embedded Postgres did not become ready in 120s" >&2
                kill -TERM "$PG_PID" 2>/dev/null || true
                exit 1
            fi
            if ! kill -0 "$PG_PID" 2>/dev/null; then
                echo "evey: embedded Postgres exited before becoming ready - check the postgres: lines above" >&2
                exit 1
            fi
            sleep 0.5
        done
    }
    pg_stop() {
        "$PG_BIN/pg_ctl" -D "$PGDATA_DIR" -m fast -s stop 2>/dev/null || kill -TERM "$PG_PID" 2>/dev/null || true
        wait "$PG_PID" 2>/dev/null || true
    }
    # The password reaches psql through its environment, never argv (world-readable in /proc).
    pg_psql() {
        PGPASSWORD="$pg_pass" "$PG_BIN/psql" -h "$PG_SOCKET_DIR" -p "$pg_port" -U "$pg_user" "$@"
    }
    # Idempotent every boot: the role password follows whatever the operator
    # (or the keys file) says NOW, and the database exists. psql variables keep
    # the values out of the SQL text (:'x' quotes a literal, :"x" an identifier).
    pg_admin() {
        pg_psql -q -d postgres -v ON_ERROR_STOP=1 -v u="$pg_user" -v db="$pg_db" <<'SQL'
\getenv pw PGPASSWORD
ALTER ROLE :"u" PASSWORD :'pw';
SELECT format('CREATE DATABASE %I OWNER %I', :'db', :'u')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'db') \gexec
SQL
    }

    pg_start 127.0.0.1
    if pg_psql -q -d postgres -tAc 'SELECT 1' >/dev/null 2>&1; then
        pg_admin
    else
        # The stored password no longer matches (EVEY_DB_PASSWORD or the keys
        # file changed): realign it from a socket-only, owner-only trust start.
        # Safe because neither the server nor any plugin exists yet.
        echo "evey: embedded Postgres refused the configured password - realigning the role password through a socket-only maintenance start"
        pg_stop
        pg_admin_hba="$PG_SOCKET_DIR/admin_hba.conf"
        (umask 077; printf 'local all all trust\n' > "$pg_admin_hba")
        pg_start '' -c hba_file="$pg_admin_hba"
        pg_admin
        pg_stop
        rm -f "$pg_admin_hba"
        pg_start 127.0.0.1
        if ! pg_psql -q -d postgres -tAc 'SELECT 1' >/dev/null; then
            echo "evey: embedded Postgres still refuses the configured password after realigning it - check EVEY_DB_USER/EVEY_DB_PASSWORD and $pg_hba" >&2
            pg_stop
            exit 1
        fi
    fi

    # ── One-shot import: moving an existing install onto the embedded DB ──
    # A plain `pg_dump | gzip` of the previous database dropped at
    # /data/postgres-import.sql.gz is restored into the cluster created THIS
    # boot, before the server runs (so its migrations then apply on top of
    # the imported schema, exactly as after an image update). Nothing is ever
    # imported into a cluster that already exists: a stale file next to a live
    # database is reported and ignored, never merged. A failed import leaves
    # no half-cluster behind - the directory is removed so the next boot
    # starts over from the (untouched) dump.
    PG_IMPORT="${EVEY_POSTGRES_IMPORT:-/data/postgres-import.sql.gz}"
    if [ -s "$PG_IMPORT" ]; then
        if [ "$pg_fresh" = "1" ]; then
            echo "evey: importing $PG_IMPORT into the new embedded database (this can take a while on a large catalogue)"
            if gunzip -c "$PG_IMPORT" | pg_psql -q -d "$pg_db" -v ON_ERROR_STOP=1 >/dev/null; then
                mv "$PG_IMPORT" "$PG_IMPORT.imported-$(date -u +%Y%m%dT%H%M%SZ)"
                echo "evey: import done - the dump was renamed *.imported-<stamp>, delete it once you have checked the server"
            else
                echo "evey: import FAILED - discarding the new cluster so the next boot retries from the dump; fix the dump (a plain pg_dump --no-owner --no-privileges of the evey database, gzipped) or remove it to start empty" >&2
                pg_stop
                rm -rf "$PGDATA_DIR"
                exit 1
            fi
        else
            echo "evey: $PG_IMPORT is present but the database already exists - NOT imported; remove the file, or move $PGDATA_DIR aside to import into a fresh cluster" >&2
        fi
    fi

    export EVEY_DB_HOST=127.0.0.1 EVEY_DB_PORT="$pg_port" EVEY_DB_SSLMODE=disable
    echo "evey: embedded Postgres $PG_MAJOR ready on 127.0.0.1:$pg_port (data in $PGDATA_DIR)"
fi

# ── Embedded Meilisearch ─────────────────────────────────────────────────
EVEY_EMBEDDED_MEILI="${EVEY_EMBEDDED_MEILI:-true}"

MEILI_PID=""
if [ "$EVEY_EMBEDDED_MEILI" = "true" ]; then
    # The key protects an index reachable only on the container's loopback -
    # an internal secret, so the image invents it (persisted next to the other
    # keys) unless the operator brings one. The server reads the same file
    # through MEILI_MASTER_KEY_FILE.
    meili_key_file="${MEILI_MASTER_KEY_FILE:-$KEYS_DIR/meili.key}"
    if [ -n "${MEILI_MASTER_KEY:-}" ]; then
        meili_key="$MEILI_MASTER_KEY"
    else
        meili_key="$(ensure_secret "$meili_key_file" "embedded Meilisearch master key")"
        export MEILI_MASTER_KEY_FILE="$meili_key_file"
    fi

    MEILI_DATA_DIR="${MEILI_DATA_DIR:-/data/meili}"
    mkdir -p "$MEILI_DATA_DIR"

    # ── Indexing memory budget (cgroup-aware) ──────────────────────────────
    # Meilisearch sizes its indexing arena from the MACHINE's RAM (defaults to
    # 2/3 of total), blind to the container limit - on a 2 GB NAS with a 1 GB
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
        echo "evey: memory budget $((limit / 1048576)) MiB detected - capping Meilisearch indexing memory at ${budget_mib} MiB"
    fi

    # --dump-dir must be explicit: it defaults to a relative "dumps/" that
    # Meilisearch creates unconditionally at startup against the process's
    # CWD, not under --db-path. This script never `cd`s anywhere, so that CWD
    # is /, unwritable by the unprivileged evey user - meilisearch exited
    # with "Permission denied (os error 13)" on the mkdir before it ever
    # reached the healthcheck. Colocating it under MEILI_DATA_DIR is both
    # writable and gets backed up together with the index.
    # The master key goes through the environment, never argv: any process
    # can read /proc/<pid>/cmdline, environ is guarded like ptrace.
    MEILI_MASTER_KEY="$meili_key" /usr/local/bin/meilisearch \
        --db-path "$MEILI_DATA_DIR" \
        --dump-dir "$MEILI_DATA_DIR/dumps" \
        --http-addr "127.0.0.1:7700" \
        --env production \
        --no-analytics &
    MEILI_PID=$!

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
            echo "evey: embedded Meilisearch exited before becoming healthy - check logs above" >&2
            exit 1
        fi
        sleep 0.5
    done

    export EVEY_MEILI_URL="http://127.0.0.1:7700"
    echo "evey: embedded Meilisearch ready on 127.0.0.1:7700"
fi

# ── Hand off to evey ─────────────────────────────────────────────────────
# This script STAYS PID 1 as a small supervisor, with or without an embedded
# database. Two things depend on it:
#
#  1. An embedded database killed with the container replays its WAL on the
#     next boot - safe but slow and needless. `docker stop` must therefore
#     reach Postgres as an orderly fast shutdown AFTER the server has let go
#     of its connections, which only a supervisor can sequence.
#  2. The admin panel's "restart server" (EVEY_RESTART_EXIT_CODE below).
#
# SIGTERM/SIGINT are forwarded to the server; when it exits for any other
# reason its exit code becomes the container's, exactly as when evey was
# exec'd into PID 1.
#
# It used to `exec /evey` when there was no embedded database, which left evey
# as PID 1 - and the restart RPC keyed its availability on `getpid() == 1`.
# The result was backwards: restart worked ONLY on the deployment shape
# without an embedded database, and was refused ("server restart is not
# available in this deployment") on the single-file install the project
# recommends. One supervised path removes both the special case and the
# heuristic.

# EVEY_RESTART_EXIT_CODE is the contract between the admin restart RPC and this
# supervisor: evey drains, exits with it, and we relaunch it in place.
#
# Doing it here rather than letting the container die and trusting Docker's
# `restart:` policy is deliberate. Nothing inside a container can read that
# policy, so relying on it means the button either works or silently stops the
# server for good, with no way to tell which beforehand. Restarting the process
# ourselves is the only guarantee available from in here - and it is also
# faster, since Postgres and Meilisearch keep running underneath.
#
# 86: outside the conventional ranges (0-2 shells, 126-127 exec failures,
# 128+n signals), so it cannot collide with a real crash.
EVEY_RESTART_EXIT_CODE=86
export EVEY_RESTART_EXIT_CODE
# Tells evey the restart RPC has somewhere to land. Absent (a bare binary, or
# any launcher that is not this script), the RPC correctly reports itself
# unavailable instead of exiting into nothing.
EVEY_SUPERVISED=1
export EVEY_SUPERVISED
# The embedded Postgres and Meilisearch are Evey's load too, but siblings of
# the server: it counts their trees beside its own in the admin CPU share.
EVEY_COMPANION_PIDS="$PG_PID $MEILI_PID"
export EVEY_COMPANION_PIDS

stop_services() {
    if [ -n "$PG_PID" ] && kill -0 "$PG_PID" 2>/dev/null; then
        "$PG_BIN/pg_ctl" -D "$PGDATA_DIR" -m fast -t 30 -s stop 2>/dev/null \
            || kill -TERM "$PG_PID" 2>/dev/null || true
        wait "$PG_PID" 2>/dev/null || true
    fi
    if [ -n "$MEILI_PID" ] && kill -0 "$MEILI_PID" 2>/dev/null; then
        kill -TERM "$MEILI_PID" 2>/dev/null || true
        wait "$MEILI_PID" 2>/dev/null || true
    fi
}

# stopping distinguishes "the operator asked us to stop" from "the server
# exited on its own". Without it, a SIGTERM that races the drain would be read
# as a plain exit and, on the restart code, relaunch a server we were told to
# shut down.
stopping=0
trap 'stopping=1; kill -TERM "$EVEY_PID" 2>/dev/null || true' TERM INT

while :; do
    /evey "$@" &
    EVEY_PID=$!
    rc=0
    # A trapped signal makes `wait` return early (128+signo) while the server is
    # still shutting down: wait again until it has really gone.
    while :; do
        wait "$EVEY_PID" && rc=0 || rc=$?
        kill -0 "$EVEY_PID" 2>/dev/null || break
    done
    if [ "$stopping" -eq 0 ] && [ "$rc" -eq "$EVEY_RESTART_EXIT_CODE" ]; then
        echo "evey: admin-requested restart - relaunching the server (database and search stay up)"
        continue
    fi
    break
done

trap - TERM INT
stop_services
exit "$rc"
