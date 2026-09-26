# Evey container: the interactive shell IS the Evey admin shell, with an
# arrow-key control centre on top. Sourced from /etc/bash.bashrc on an
# interactive shell.
#
#  - REAL bash underneath: the prompt is branded `evey ▸`, native `evey` /
#    `mkvgo` Tab-completion works, any command / pipe / glob runs. Short admin
#    verbs (doctor, migrate, reindex-search, ...) map 1:1 to `evey admin <verb>`.
#  - The launcher is a menu tree:
#      main (evey / mkvgo / shell) -> submenu -> options
#    Enter drills in; ready-to-run options run in place (screen laid out
#    logo / rule / output / rule / menu). An option needing a value lands
#    PRE-TYPED on the real prompt - full completion (verbs, flags, paths,
#    --user emails) - and once that command finishes the launcher reopens
#    by itself in the submenu it came from (PROMPT_COMMAND hook). Tab
#    anywhere hands the highlighted example to the prompt the same way.
#    q/Esc backs up one level (options -> submenu -> main -> plain shell).
#    `menu` reopens it, `?` prints a static cheat-sheet, `exit` leaves.
#  - No tty (piped / logs / non-interactive): a plain list, no key capture.
#  - Want a vanilla bash instead? `EVEY_SHELL=plain` (or `EVEY_MENU=off`) skips
#    all of it -- no branding, no launcher, no verbs. e.g.
#      docker exec -it -e EVEY_SHELL=plain evey-server bash

_EVEY_VERBS='reset-password rotate-jwt-key restore-backup reindex-search migrate reclassify reindex-no-cue validate-streaming artwork-gc load-test doctor rollback ladder-bench ladder-probe route-bench list-users list-libraries'

# ---------------------------------------------------------------------------
# Look & feel
# ---------------------------------------------------------------------------

# Sets colour vars (empty when not a tty / NO_COLOR).
_evey_palette() {
    if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
        W=$'\033[38;2;236;238;244m'; R=$'\033[38;2;220;38;38m'; S=$'\033[38;2;136;145;163m'
        D=$'\033[38;2;120;128;145m'; G=$'\033[38;2;108;192;139m'; Z=$'\033[0m'
    else
        W=''; R=''; S=''; D=''; G=''; Z=''
    fi
}

_evey_banner() {
    local W R S D G Z; _evey_palette
    printf '\n'
    printf '  %s███████╗%s██╗   ██╗%s███████╗%s██╗   ██╗%s\n' "$W" "$R" "$W" "$W" "$Z"
    printf '  %s██╔════╝%s██║   ██║%s██╔════╝%s╚██╗ ██╔╝%s\n' "$W" "$R" "$W" "$W" "$Z"
    printf '  %s█████╗  %s██║   ██║%s█████╗  %s ╚████╔╝ %s\n' "$W" "$R" "$W" "$W" "$Z"
    printf '  %s██╔══╝  %s╚██╗ ██╔╝%s██╔══╝  %s  ╚██╔╝  %s\n' "$W" "$R" "$W" "$W" "$Z"
    printf '  %s███████╗%s ╚████╔╝ %s███████╗%s   ██║   %s\n' "$W" "$R" "$W" "$W" "$Z"
    printf '  %s╚══════╝%s  ╚═══╝  %s╚══════╝%s   ╚═╝   %s\n' "$W" "$R" "$W" "$W" "$Z"
    if _evey_narrow; then
        printf '  %sadmin shell%s %s·%s type %smenu%s\n\n' "$D" "$Z" "$D" "$Z" "$R" "$Z"
    else
        printf '  %sadmin shell%s   %s·%s   %stab-completion on%s   %s·%s   type %smenu%s %sto come back%s\n\n' \
            "$D" "$Z" "$D" "$Z" "$G" "$Z" "$D" "$Z" "$R" "$Z" "$D" "$Z"
    fi
}

# Terminal width, cheap (no fork): bash keeps COLUMNS current on resize in
# interactive shells (checkwinsize). Fallback for odd environments.
_evey_cols() {
    printf '%s' "${COLUMNS:-$(tput cols 2>/dev/null || echo 80)}"
}

# Narrow screen (phone SSH)? The long taglines/hints get short variants.
_evey_narrow() { [ "$(_evey_cols)" -lt 68 ]; }

# The horizontal separator drawn around command output, clipped to the
# terminal so it never wraps on a narrow screen (phone SSH).
_evey_rule() {
    local W R S D G Z; _evey_palette
    local width=$(( $(_evey_cols) - 4 )); [ "$width" -gt 56 ] && width=56; [ "$width" -lt 8 ] && width=8
    local line=''
    while [ "${#line}" -lt "$width" ]; do line="${line}─"; done
    printf '  %s%s%s\n' "$D" "$line" "$Z"
}

# ---------------------------------------------------------------------------
# Menu data. Each loader fills label/prefill/description arrays; every list
# ends with a nav row (prefill '') the painters render as '← back' / 'shell'.
# ---------------------------------------------------------------------------

_evey_load_main_menu() {
    main_labels=( 'evey' 'mkvgo' 'shell' )
    main_descs=( 'server admin toolbox - doctor, migrate, reindex, users, ...' \
                 'media file toolbox - inspect / repair / convert' \
                 'drop to the real prompt (menu reopens it)' )
}

# The evey admin verbs. The same arrays feed the launcher, the no-tty list
# and the `?` cheat-sheet.
_evey_load_admin_menu() {
    admin_labels=(   'doctor' 'migrate' 'reset-password' 'rotate-jwt-key' 'restore-backup' \
                     'reindex-search' 'reclassify' 'reindex-no-cue' 'validate-streaming' \
                     'artwork-gc' 'load-test' 'rollback' 'route-bench' 'ladder-bench' 'ladder-probe' \
                     'list-users' 'list-libraries' '← back' )
    admin_prefills=( 'doctor' 'migrate ' 'reset-password --user ' 'rotate-jwt-key' 'restore-backup ' \
                     'reindex-search' 'reclassify ' 'reindex-no-cue --root ' 'validate-streaming --root ' \
                     'artwork-gc' 'load-test --scenario balanced --max-viewers ' 'rollback' 'route-bench ' \
                     'ladder-bench ' 'ladder-probe ' 'list-users' 'list-libraries' '' )
    admin_descs=(    'health-check everything (--repair fixes safe stuff)' \
                     'schema: up | down | version' \
                     'generate a new password, print it once' \
                     'regenerate the JWT key - logs EVERYONE out' \
                     'restore the database from a pg_dump.gz' \
                     'rebuild the search index' \
                     'recompute the encoding class (<media-id> | --all)' \
                     'repair MKV seek indexes (dry-run; --apply writes)' \
                     'report seek / HLS readiness of a library' \
                     'delete orphaned artwork (dry-run; --apply deletes)' \
                     'ramp viewers, find the capacity ceiling' \
                     'read-only advisor: version, schema, newest backup' \
                     'cost of each transcode route on this host (heavy, never while serving)' \
                     'ladder complexity features on a few scenes (heavy, never while serving)' \
                     'per-title probe timing on given files (encodes, never while serving)' \
                     'print all user emails' \
                     'print the library root paths' \
                     'to the main menu' )
}

# The mkvgo sub-commands surfaced in the launcher (all take a file argument ->
# Enter opens the inline line to fill it). The full list stays available via
# native completion (`mkvgo <Tab>`) and the free-form row.
_evey_load_mkvgo_menu() {
    local version; version=$(mkvgo --version 2>/dev/null | awk '{print $2}'); [ -n "$version" ] || version='?'
    mkvgo_labels=(   'info' 'probe' 'diagnose' 'validate' 'verify' 'playability' 'analyze' 'keyframes' \
                     'reindex' 'reindex-inplace' 'salvage' 'to-mp4' 'mkvgo <cmd>' '← back' )
    mkvgo_prefills=( 'mkvgo info ' 'mkvgo probe ' 'mkvgo diagnose ' 'mkvgo validate ' 'mkvgo verify ' \
                     'mkvgo playability -target chrome ' 'mkvgo analyze ' 'mkvgo keyframes ' \
                     'mkvgo reindex ' 'mkvgo reindex-inplace ' 'mkvgo salvage ' 'mkvgo to-mp4 ' \
                     'mkvgo ' '' )
    mkvgo_descs=(    'tracks, codecs, duration' \
                     'deep structure probe (-json for machines)' \
                     'one-call triage with a remedy per finding' \
                     'structural validation' \
                     'deep A/V integrity check' \
                     'direct-play report for a browser target' \
                     'content scan - CFR/VFR, complexity' \
                     'list keyframe positions' \
                     'rebuild the seek index (Cues) to a new file' \
                     'rebuild Cues in place (keeps a backup)' \
                     'recover a damaged / truncated file' \
                     'remux MKV -> MP4' \
                     "any other sub-command  ($version)" \
                     'to the main menu' )
}

# Follow-up options for one admin verb (keyed by NAME, not list position).
# option_kinds[i]: "run" executes as-is, "prompt" opens the inline line to
# fill a value. Returns 1 for anything that has no options screen.
_evey_load_admin_options() {
    option_labels=(); option_commands=(); option_kinds=()
    case "$1" in
        doctor)
            option_labels=('run (no flags)' '--repair (fixes safe stuff)')
            option_commands=('doctor'       'doctor --repair')
            option_kinds=(  'run'           'run') ;;
        migrate)
            option_labels=('up' 'down (one step back)' 'version')
            option_commands=('migrate up' 'migrate down' 'migrate version')
            option_kinds=(  'run' 'run' 'run') ;;
        reset-password)
            option_labels=('--user <email>')
            option_commands=('reset-password --user ')
            option_kinds=(  'prompt') ;;
        rotate-jwt-key)
            option_labels=('run (logs EVERYONE out)')
            option_commands=('rotate-jwt-key')
            option_kinds=(  'run') ;;
        restore-backup)
            option_labels=('<path-to-dump.sql.gz>')
            option_commands=('restore-backup ')
            option_kinds=(  'prompt') ;;
        reindex-search)
            option_labels=('run (all libraries)' '--library <id>' '--purge (rebuild from scratch)')
            option_commands=('reindex-search'    'reindex-search --library ' 'reindex-search --purge')
            option_kinds=(  'run'                'prompt'                    'run') ;;
        reclassify)
            option_labels=('<media-id>'  '--all (re-queue everything)')
            option_commands=('reclassify ' 'reclassify --all')
            option_kinds=(  'prompt'     'run') ;;
        reindex-no-cue)
            option_labels=('--root <dir> (dry-run)' '--root <dir> --apply (writes)' '--root <dir> --resync --apply (repairs corrupted)')
            option_commands=('reindex-no-cue --root ' 'reindex-no-cue --apply --root ' 'reindex-no-cue --resync --apply --root ')
            option_kinds=(  'prompt'                  'prompt'                        'prompt') ;;
        validate-streaming)
            option_labels=('--root <dir>'              '--root <dir> --errors-only')
            option_commands=('validate-streaming --root ' 'validate-streaming --errors-only --root ')
            option_kinds=(  'prompt'                   'prompt') ;;
        artwork-gc)
            option_labels=('run (dry-run)' '--apply (deletes)' '--root <dir>')
            option_commands=('artwork-gc'  'artwork-gc --apply' 'artwork-gc --root ')
            option_kinds=(  'run'          'run'                'prompt') ;;
        load-test)
            option_labels=('run (defaults: balanced, server cap)' '--scenario balanced --max-viewers <n>')
            option_commands=('load-test'                          'load-test --scenario balanced --max-viewers ')
            option_kinds=(  'run'                                 'prompt') ;;
        rollback)
            option_labels=('run (read-only)')
            option_commands=('rollback')
            option_kinds=(  'run') ;;
        list-users)
            option_labels=('run')
            option_commands=('list-users')
            option_kinds=(  'run') ;;
        list-libraries)
            option_labels=('run')
            option_commands=('list-libraries')
            option_kinds=(  'run') ;;
        route-bench)
            option_labels=('synthetic clips only (auto-detect the GPU)' '--backend none (CPU routes only)' '<file>... (real titles too)')
            option_commands=('route-bench'                              'route-bench --backend none'       'route-bench ')
            option_kinds=(  'run'                                       'run'                             'prompt') ;;
        ladder-bench)
            option_labels=('<file>... (features + CRF encodes)' '--efficiency <file>... (encoder curves)')
            option_commands=('ladder-bench '                    'ladder-bench --efficiency ')
            option_kinds=(  'prompt'                            'prompt') ;;
        ladder-probe)
            option_labels=('<file>...')
            option_commands=('ladder-probe ')
            option_kinds=(  'prompt') ;;
        *)  return 1 ;;
    esac
}

# Follow-up options for one mkvgo sub-command (flags verified against the
# binary's usage strings). All need a file -> every option is prompt-kind.
_evey_load_mkvgo_options() {
    option_labels=(); option_commands=(); option_kinds=()
    case "$1" in
        info|probe|diagnose|validate|keyframes)
            option_labels=('<file>'      '-json <file>')
            option_commands=("mkvgo $1 " "mkvgo $1 -json ")
            option_kinds=(  'prompt'     'prompt') ;;
        verify|analyze)
            option_labels=('<file>')
            option_commands=("mkvgo $1 ")
            option_kinds=(  'prompt') ;;
        playability)
            option_labels=('-target chrome <file>' '-target safari <file>' '-target chromecast-gen3 <file>' '-target <other> <file>')
            option_commands=('mkvgo playability -target chrome ' 'mkvgo playability -target safari ' \
                             'mkvgo playability -target chromecast-gen3 ' 'mkvgo playability -target ')
            option_kinds=(  'prompt' 'prompt' 'prompt' 'prompt') ;;
        reindex)
            option_labels=('<in> [out] (new file, verified)' '--replace <file> (swap in, keeps backup)' '--resync --replace <file> (repairs corrupted)' '--deep-verify <in> [out]')
            option_commands=('mkvgo reindex ' 'mkvgo reindex --replace ' 'mkvgo reindex --resync --replace ' 'mkvgo reindex --deep-verify ')
            option_kinds=(  'prompt' 'prompt' 'prompt' 'prompt') ;;
        reindex-inplace)
            option_labels=('<file> (crash-safe journal)' '--deep-verify <file>')
            option_commands=('mkvgo reindex-inplace ' 'mkvgo reindex-inplace --deep-verify ')
            option_kinds=(  'prompt' 'prompt') ;;
        salvage)
            option_labels=('<in> <out>' '--dry-run <in> (damage map only)')
            option_commands=('mkvgo salvage ' 'mkvgo salvage --dry-run ')
            option_kinds=(  'prompt' 'prompt') ;;
        to-mp4)
            option_labels=('<in> <out>' '--faststart <in> <out>')
            option_commands=('mkvgo to-mp4 ' 'mkvgo to-mp4 --faststart ')
            option_kinds=(  'prompt' 'prompt') ;;
        *)  return 1 ;;
    esac
}

# `?` -> a quick, non-modal reference.
evey_help() {
    local W R S D G Z; _evey_palette
    local -a admin_labels admin_prefills admin_descs; _evey_load_admin_menu
    _evey_banner
    local i
    for ((i=0; i<${#admin_labels[@]}-1; i++)); do
        printf '  %s%-20s%s %s%s%s\n' "$R" "${admin_labels[i]}" "$Z" "$D" "${admin_descs[i]}" "$Z"
    done
    printf '\n  %smkvgo <cmd> for media files%s  ·  %smenu%s launcher  ·  %sexit%s leaves\n' \
        "$S" "$Z" "$R" "$Z" "$R" "$Z"
    printf '  %sfull form works too: %sevey admin <verb>%s%s\n\n' "$D" "$W" "$Z" "$Z"
}

# ---------------------------------------------------------------------------
# Engine
# ---------------------------------------------------------------------------

# One keypress -> a verb in REPLY (up/down/enter/edit/back/other). Arrow
# escape sequences are read with a short timeout so a BARE Esc means "back".
_evey_read_key() {
    local key rest
    IFS= read -rsn1 key 2>/dev/null || { REPLY=back; return; }
    if [ "$key" = $'\033' ]; then
        rest=''
        read -rsn2 -t 0.05 rest 2>/dev/null
        [ -z "$rest" ] && { REPLY=back; return; }
        key+="$rest"
    fi
    case "$key" in
        $'\033[A'|$'\033OA'|k) REPLY=up ;;
        $'\033[B'|$'\033OB'|j) REPLY=down ;;
        $'\t') REPLY=edit ;;
        ''|$'\n'|$'\r') REPLY=enter ;;
        q|Q) REPLY=back ;;
        *) REPLY=other ;;
    esac
}

# Run one command with the screen laid out logo / rule / output / rule, then
# flag the next menu draw to reuse the banner already on screen.
_evey_run_command() {
    printf '\033[H\033[2J'
    _evey_banner
    _evey_rule
    printf '\n'
    eval "$1"
    printf '\n'
    _evey_rule
    printf '\n'
    _EVEY_SKIP_BANNER=1
}

# Hand a command to the REAL prompt (red `evey ▸`, FULL programmable
# completion: verbs, flags, paths, --user emails), pre-typed and editable.
# The DSR reply (\e[0n) triggers the bound macro once the prompt is drawn.
# $2 = the launcher level the prompt hook reopens at once the command ran.
# _EVEY_RESUME counts the prompts still to pass before reopening: the prompt
# that carries the pre-typed text must be skipped, so 2 from a plain call. A
# hand-off made from INSIDE the hook is already inside that prompt's
# PROMPT_COMMAND, so only the post-command one is left -> 1.
_evey_handoff_command() {
    local W R S D G Z; _evey_palette
    _EVEY_RESUME_LEVEL="$2"
    if [ -n "${_EVEY_IN_HOOK:-}" ]; then _EVEY_RESUME=1; else _EVEY_RESUME=2; fi
    if _evey_narrow; then
        printf '  %srun/edit it - menu comes back after%s\n' "$D" "$Z"
    else
        printf '  %srun it, edit it, or type something else - the menu comes back after%s\n' "$D" "$Z"
    fi
    [ -n "$1" ] || return 0     # empty prefill: a bare prompt, same comeback
    history -s "$1"
    bind "\"\033[0n\": \"$1\"" 2>/dev/null && printf '\033[5n'
}

# PROMPT_COMMAND hook: reopen the launcher (framed by a rule) at the level it
# was handed off from, once the handed-off command has run.
_evey_prompt_hook() {
    [ -n "${_EVEY_RESUME:-}" ] || return 0
    if [ "$_EVEY_RESUME" -gt 1 ]; then _EVEY_RESUME=$((_EVEY_RESUME - 1)); return 0; fi
    unset _EVEY_RESUME
    local level="${_EVEY_RESUME_LEVEL:-main}"; unset _EVEY_RESUME_LEVEL
    _evey_rule
    _EVEY_IN_HOOK=1
    evey_menu "$level"
    unset _EVEY_IN_HOOK
}

# `menu` -> the launcher. See the header comment for the full behaviour.
evey_menu() {
    local W R S D G Z; _evey_palette
    local -a main_labels main_descs admin_labels admin_prefills admin_descs
    local -a mkvgo_labels mkvgo_prefills mkvgo_descs
    local -a option_labels option_commands option_kinds
    _evey_load_main_menu; _evey_load_admin_menu; _evey_load_mkvgo_menu

    # No tty: static list, no key capture.
    if [ ! -t 0 ] || [ ! -t 1 ]; then
        _evey_banner
        local i
        for ((i=0; i<${#admin_labels[@]}-1; i++)); do
            printf '  %s▸%s %s%-20s%s %s%s%s\n' "$R" "$Z" "$W" "${admin_labels[i]}" "$Z" "$D" "${admin_descs[i]}" "$Z"
        done
        printf '\n  %sTab-complete: %sevey admin%s %s<Tab>%s  ·  %smkvgo%s %s<Tab>%s\n\n' \
            "$D" "$R" "$Z" "$D" "$Z" "$R" "$Z" "$D" "$Z"
        return 0
    fi

    local main_sel=0 admin_sel=0 mkvgo_sel=0 option_sel=0 i
    local level="${1:-main}" option_verb='' option_source=admin
    local main_count=${#main_labels[@]} admin_count=${#admin_labels[@]} mkvgo_count=${#mkvgo_labels[@]}

    # Row painters; $1 = row index. The selected row gets the ► mark. The
    # nav row pads to 22 ('←' is 3 UTF-8 bytes and %-*s counts bytes).
    #
    # EVERY row is clipped to the terminal width: the in-place repaint below
    # moves the cursor up by the LOGICAL row count, so one logical row MUST
    # occupy exactly one physical line. On a narrow terminal (phone SSH,
    # Termux portrait) an unclipped description wraps, the cursor-up lands
    # mid-list, and every keypress reprints the whole menu further down -
    # the "menu re-printing in a loop" bug.
    _evey_clip_desc() {
        # $1 = columns already used by mark+label+space; prints the clipped
        # description for $2 (or nothing when the screen is too narrow).
        local avail=$(( $(_evey_cols) - $1 - 1 ))
        if [ "$avail" -lt 8 ]; then printf ''; else printf '%s' "${2:0:avail}"; fi
    }
    _evey_paint_main_row() {
        local i="$1" mark col
        if [ "$i" -eq "$main_sel" ]; then mark="${R}►${Z} "; col="$W"; else mark="  "; col="$D"; fi
        printf '%s%s%-20s%s %s%s%s\n' "$mark" "$col" "${main_labels[i]}" "$Z" "$D" "$(_evey_clip_desc 23 "${main_descs[i]}")" "$Z"
    }
    _evey_paint_admin_row() {
        local i="$1" mark col width=20
        if [ "$i" -eq "$admin_sel" ]; then mark="${R}►${Z} "; col="$W"; else mark="  "; col="$D"; fi
        [ "$i" -eq $((admin_count-1)) ] && width=22
        printf '%s%s%-*s%s %s%s%s\n' "$mark" "$col" "$width" "${admin_labels[i]}" "$Z" "$D" "$(_evey_clip_desc 23 "${admin_descs[i]}")" "$Z"
    }
    _evey_paint_mkvgo_row() {
        local i="$1" mark col width=20
        if [ "$i" -eq "$mkvgo_sel" ]; then mark="${R}►${Z} "; col="$W"; else mark="  "; col="$D"; fi
        [ "$i" -eq $((mkvgo_count-1)) ] && width=22
        printf '%s%s%-*s%s %s%s%s\n' "$mark" "$col" "$width" "${mkvgo_labels[i]}" "$Z" "$D" "$(_evey_clip_desc 23 "${mkvgo_descs[i]}")" "$Z"
    }
    _evey_paint_option_row() {
        local i="$1" mark col label
        if [ "$i" -eq "$option_sel" ]; then mark="${R}►${Z} "; col="$W"; else mark="  "; col="$D"; fi
        label="${option_labels[i]}"
        label="${label:0:$(( $(_evey_cols) - 4 ))}"
        printf '%s%s%s%s\n' "$mark" "$col" "$label" "$Z"
    }

    # Banner for a fresh screen - skipped once right after a run, whose own
    # banner (above the output) is already the header of this screen.
    _evey_paint_banner() {
        if [ "${_EVEY_SKIP_BANNER:-0}" = 1 ]; then _EVEY_SKIP_BANNER=0; else _evey_banner; fi
    }

    # Key loop shared by every level: capture keys (cursor hidden - the
    # blinking bar under a list is noise), repaint rows in place on movement,
    # hand the terminating verb back through REPLY.
    # $1 = row painter  $2 = row count  $3 = NAME of the selection variable.
    _evey_navigate() {
        local painter="$1" count="$2" sel_var="$3"
        tput civis 2>/dev/null
        while true; do
            _evey_read_key
            case "$REPLY" in
                up)   eval "$sel_var=\$(( (\$$sel_var - 1 + count) % count ))" ;;
                down) eval "$sel_var=\$(( (\$$sel_var + 1) % count ))" ;;
                enter|edit|back) break ;;
                *) continue ;;
            esac
            printf '\033[%dA' "$count"
            for ((i=0; i<count; i++)); do printf '\033[2K'; "$painter" "$i"; done
        done
        tput cnorm 2>/dev/null
        printf '\n'
    }

    while true; do
        case "$level" in
        main)
            _evey_paint_banner
            if _evey_narrow; then
                printf '  %s↑/↓ · Enter · Tab cmd · q/Esc shell%s\n' "$D" "$Z"
            else
                printf '  %s↑/↓ move · Enter open · Tab type a command · q/Esc drop to shell%s\n' "$D" "$Z"
            fi
            for ((i=0; i<main_count; i++)); do _evey_paint_main_row "$i"; done
            _evey_navigate _evey_paint_main_row "$main_count" main_sel

            if [ "$REPLY" = back ]; then break; fi
            if [ "$REPLY" = edit ]; then _evey_handoff_command '' main; break; fi
            case "${main_labels[main_sel]}" in
                evey)  level=admin ;;
                mkvgo) level=mkvgo ;;
                shell) break ;;
            esac ;;
        admin)
            _evey_paint_banner
            if _evey_narrow; then
                printf '  %sevey%s · ↑/↓ · Enter · q back%s\n' "$W" "$D" "$Z"
            else
                printf '  %sevey%s · ↑/↓ move · Enter options · Tab type/edit · q/Esc back%s\n' "$W" "$D" "$Z"
            fi
            for ((i=0; i<admin_count; i++)); do _evey_paint_admin_row "$i"; done
            _evey_navigate _evey_paint_admin_row "$admin_count" admin_sel

            if [ "$REPLY" = back ] || [ "$admin_sel" -eq $((admin_count-1)) ]; then level=main; continue; fi
            if [ "$REPLY" = edit ]; then _evey_handoff_command "${admin_prefills[admin_sel]}" admin; break; fi
            option_verb="${admin_labels[admin_sel]}"
            option_source=admin
            if _evey_load_admin_options "$option_verb"; then option_sel=0; level=options
            else _evey_handoff_command "${admin_prefills[admin_sel]}" admin; break; fi ;;
        mkvgo)
            _evey_paint_banner
            if _evey_narrow; then
                printf '  %smkvgo%s · ↑/↓ · Enter · q back%s\n' "$W" "$D" "$Z"
            else
                printf '  %smkvgo%s · ↑/↓ move · Enter options · Tab type/edit · q/Esc back%s\n' "$W" "$D" "$Z"
            fi
            for ((i=0; i<mkvgo_count; i++)); do _evey_paint_mkvgo_row "$i"; done
            _evey_navigate _evey_paint_mkvgo_row "$mkvgo_count" mkvgo_sel

            if [ "$REPLY" = back ] || [ "$mkvgo_sel" -eq $((mkvgo_count-1)) ]; then level=main; continue; fi
            if [ "$REPLY" = edit ]; then _evey_handoff_command "${mkvgo_prefills[mkvgo_sel]}" mkvgo; break; fi
            option_verb="${mkvgo_labels[mkvgo_sel]}"
            option_source=mkvgo
            if _evey_load_mkvgo_options "$option_verb"; then option_sel=0; level=options
            else _evey_handoff_command "${mkvgo_prefills[mkvgo_sel]}" mkvgo; break; fi ;;
        options)
            local option_crumb=evey
            if [ "$option_source" = mkvgo ]; then
                _evey_load_mkvgo_options "$option_verb"; option_crumb=mkvgo
            else
                _evey_load_admin_options "$option_verb"
            fi
            local option_count=${#option_labels[@]}
            if _evey_narrow; then
                printf '  %s%s ▸ %s%s · ↑/↓ · Enter · q back%s\n' \
                    "$W" "$option_crumb" "$option_verb" "$D" "$Z"
            else
                printf '  %s%s ▸ %s%s · ↑/↓ move · Enter run/fill · Tab edit · q/Esc back%s\n' \
                    "$W" "$option_crumb" "$option_verb" "$D" "$Z"
            fi
            for ((i=0; i<option_count; i++)); do _evey_paint_option_row "$i"; done
            _evey_navigate _evey_paint_option_row "$option_count" option_sel

            case "$REPLY" in
                back) level="$option_source" ;;
                edit) _evey_handoff_command "${option_commands[option_sel]}" "$option_source"; break ;;
                enter)
                    if [ "${option_kinds[option_sel]}" = run ]; then
                        history -s "${option_commands[option_sel]}"
                        _evey_run_command "${option_commands[option_sel]}"
                        level="$option_source"
                    else
                        _evey_handoff_command "${option_commands[option_sel]}" "$option_source"; break
                    fi ;;
            esac ;;
        esac
    done
    unset -f _evey_paint_main_row _evey_paint_admin_row _evey_paint_mkvgo_row \
             _evey_paint_option_row _evey_paint_banner _evey_navigate
    unset _EVEY_SKIP_BANNER
    return 0
}
menu() { evey_menu; }

# ---------------------------------------------------------------------------
# Interactive-shell wiring
# ---------------------------------------------------------------------------
case $- in
    *i*)
        # Opt-out: a plain, unbranded bash (no launcher, no verbs, default prompt).
        if [ "${EVEY_SHELL:-}" = plain ] || [ "${EVEY_MENU:-}" = off ]; then
            return 0 2>/dev/null || true
        fi

        # Short admin verbs as shell functions (`command` avoids any name clash).
        for __v in $_EVEY_VERBS; do
            eval "${__v}() { command evey admin ${__v} \"\$@\"; }"
        done
        unset __v

        # Reuse the real `evey` completion for the shorthands: rebuild the words
        # as if the user had typed `evey admin <verb> ...`, then delegate.
        _evey_shorthand() {
            local sub="${COMP_WORDS[0]}"
            COMP_WORDS=(evey admin "$sub" "${COMP_WORDS[@]:1}")
            (( COMP_CWORD += 2 ))
            _evey
        }
        if declare -F _evey >/dev/null 2>&1; then
            # shellcheck disable=SC2086
            complete -F _evey_shorthand $_EVEY_VERBS 2>/dev/null
        fi

        # `?` is a quick synonym for the cheat-sheet (aliases only expand here).
        alias '?'='evey_help' 2>/dev/null

        # The image ships no man binary: `man evey` pages the manual the
        # binary renders itself.
        man() {
            case "${1:-}" in
                evey|evey-admin) command evey man --text | "${PAGER:-more}" ;;
                *) printf 'this image ships no manual pages; `man evey` is the one it has (mkvgo --help for the media tool)\n' >&2; return 1 ;;
            esac
        }

        # zsh-style completion, everywhere the same readline runs (the real
        # prompt AND the launcher's inline command line): first Tab lists the
        # candidates and completes the common prefix, further Tabs cycle
        # through them (Shift-Tab backwards), so a --root path drills down
        # one directory level per Tab like oh-my-zsh.
        bind 'set show-all-if-ambiguous on' 2>/dev/null
        bind 'set menu-complete-display-prefix on' 2>/dev/null
        bind 'set colored-stats on' 2>/dev/null
        bind 'set mark-symlinked-directories on' 2>/dev/null
        bind '"\t": menu-complete' 2>/dev/null
        bind '"\e[Z": menu-complete-backward' 2>/dev/null

        # Reopen the launcher after a handed-off command finishes (see
        # _evey_handoff_command / _evey_prompt_hook), preserving any
        # PROMPT_COMMAND a base image may have set.
        PROMPT_COMMAND="_evey_prompt_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}"

        # Brand the prompt.
        if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
            PS1='\[\033[38;2;220;38;38m\]evey\[\033[0m\] \[\033[38;2;136;145;163m\]▸\[\033[0m\] '
        else
            PS1='evey ▸ '
        fi

        # Greet once per shell: open the launcher.
        if [ -z "${EVEY_SHELL_GREETED:-}" ]; then
            export EVEY_SHELL_GREETED=1
            evey_menu
        fi
        ;;
esac
