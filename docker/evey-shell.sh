# Evey container: the interactive shell IS the Evey admin shell, with an
# arrow-key control centre on top. Sourced from /etc/bash.bashrc on an
# interactive shell.
#
#  - REAL bash underneath: the prompt is branded `evey ▸`, native `evey` /
#    `mkvgo` Tab-completion works, any command / pipe / glob runs. Short admin
#    verbs (doctor, migrate, reindex-search, ...) map 1:1 to `evey admin <verb>`.
#  - The launcher is a menu tree you never fall out of:
#      main (evey / mkvgo / shell) -> submenu -> options
#    Enter drills in; an option needing a value opens an inline editable
#    command line right there (the example lands pre-typed; edit or retype it
#    entirely, Tab completes paths, Enter runs, an emptied line cancels).
#    Tab anywhere opens that same line seeded with the highlighted example.
#    A run redraws the screen as logo / rule / output / rule / menu, landing
#    back in the submenu it started from - the logo never scrolls away.
#    q/Esc backs up one level (options -> submenu -> main -> plain shell).
#    `menu` reopens it, `?` prints a static cheat-sheet, `exit` leaves.
#  - No tty (piped / logs / non-interactive): a plain list, no key capture.
#  - Want a vanilla bash instead? `EVEY_SHELL=plain` (or `EVEY_MENU=off`) skips
#    all of it -- no branding, no launcher, no verbs. e.g.
#      docker exec -it -e EVEY_SHELL=plain evey-server bash

_EVEY_VERBS='reset-password rotate-jwt-key restore-backup reindex-search migrate reclassify reindex-no-cue validate-streaming artwork-gc load-test doctor rollback list-users'

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
    printf '  %sadmin shell%s   %s·%s   %stab-completion on%s   %s·%s   type %smenu%s %sto come back%s\n\n' \
        "$D" "$Z" "$D" "$Z" "$G" "$Z" "$D" "$Z" "$R" "$Z" "$D" "$Z"
}

# The horizontal separator drawn around command output.
_evey_rule() {
    local W R S D G Z; _evey_palette
    printf '  %s────────────────────────────────────────────────────────%s\n' "$D" "$Z"
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
                     'artwork-gc' 'load-test' 'rollback' 'list-users' '← back' )
    admin_prefills=( 'doctor' 'migrate ' 'reset-password --user ' 'rotate-jwt-key' 'restore-backup ' \
                     'reindex-search' 'reclassify ' 'reindex-no-cue --root ' 'validate-streaming --root ' \
                     'artwork-gc' 'load-test --scenario balanced --max-viewers ' 'rollback' 'list-users' '' )
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
                     'print all user emails' \
                     'to the main menu' )
}

# The mkvgo sub-commands surfaced in the launcher (all take a file argument ->
# Enter opens the inline line to fill it). The full list stays available via
# native completion (`mkvgo <Tab>`) and the free-form row.
_evey_load_mkvgo_menu() {
    local version; version=$(mkvgo --version 2>/dev/null | awk '{print $2}'); [ -n "$version" ] || version='?'
    mkvgo_labels=(   'info' 'probe' 'validate' 'verify' 'playability' 'analyze' 'keyframes' \
                     'reindex' 'reindex-inplace' 'salvage' 'to-mp4' 'mkvgo <cmd>' '← back' )
    mkvgo_prefills=( 'mkvgo info ' 'mkvgo probe ' 'mkvgo validate ' 'mkvgo verify ' \
                     'mkvgo playability -target chrome ' 'mkvgo analyze ' 'mkvgo keyframes ' \
                     'mkvgo reindex ' 'mkvgo reindex-inplace ' 'mkvgo salvage ' 'mkvgo to-mp4 ' \
                     'mkvgo ' '' )
    mkvgo_descs=(    'tracks, codecs, duration' \
                     'deep structure probe (-json for machines)' \
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
_evey_load_options() {
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

# The inline command line: the example lands PRE-TYPED and fully editable
# (readline: arrows, backspace, Tab completes paths) right under the menu -
# no dropping out to a bare prompt. Enter runs it; an emptied line cancels.
# Returns 0 if a command ran, 1 on cancel.
_evey_prompt_command() {
    local W R S D G Z; _evey_palette
    local line
    printf '  %sedit freely · Enter runs · emptied line cancels%s\n' "$D" "$Z"
    if ! IFS= read -e -r -p '  evey ▸ ' -i "$1" line; then printf '\n'; return 1; fi
    [ -n "${line//[[:space:]]/}" ] || return 1
    history -s "$line"
    _evey_run_command "$line"
    return 0
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
    local level=main option_verb=''
    local main_count=${#main_labels[@]} admin_count=${#admin_labels[@]} mkvgo_count=${#mkvgo_labels[@]}

    # Row painters; $1 = row index. The selected row gets the ► mark. The
    # nav row pads to 22 ('←' is 3 UTF-8 bytes and %-*s counts bytes).
    _evey_paint_main_row() {
        local i="$1" mark col
        if [ "$i" -eq "$main_sel" ]; then mark="${R}►${Z} "; col="$W"; else mark="  "; col="$D"; fi
        printf '%s%s%-20s%s %s%s%s\n' "$mark" "$col" "${main_labels[i]}" "$Z" "$D" "${main_descs[i]}" "$Z"
    }
    _evey_paint_admin_row() {
        local i="$1" mark col width=20
        if [ "$i" -eq "$admin_sel" ]; then mark="${R}►${Z} "; col="$W"; else mark="  "; col="$D"; fi
        [ "$i" -eq $((admin_count-1)) ] && width=22
        printf '%s%s%-*s%s %s%s%s\n' "$mark" "$col" "$width" "${admin_labels[i]}" "$Z" "$D" "${admin_descs[i]}" "$Z"
    }
    _evey_paint_mkvgo_row() {
        local i="$1" mark col width=20
        if [ "$i" -eq "$mkvgo_sel" ]; then mark="${R}►${Z} "; col="$W"; else mark="  "; col="$D"; fi
        [ "$i" -eq $((mkvgo_count-1)) ] && width=22
        printf '%s%s%-*s%s %s%s%s\n' "$mark" "$col" "$width" "${mkvgo_labels[i]}" "$Z" "$D" "${mkvgo_descs[i]}" "$Z"
    }
    _evey_paint_option_row() {
        local i="$1" mark col
        if [ "$i" -eq "$option_sel" ]; then mark="${R}►${Z} "; col="$W"; else mark="  "; col="$D"; fi
        printf '%s%s%s%s\n' "$mark" "$col" "${option_labels[i]}" "$Z"
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
            printf '  %s↑/↓ move · Enter open · Tab type a command · q/Esc drop to shell%s\n' "$D" "$Z"
            for ((i=0; i<main_count; i++)); do _evey_paint_main_row "$i"; done
            _evey_navigate _evey_paint_main_row "$main_count" main_sel

            if [ "$REPLY" = back ]; then break; fi
            if [ "$REPLY" = edit ]; then _evey_prompt_command '' || true; continue; fi
            case "${main_labels[main_sel]}" in
                evey)  level=admin ;;
                mkvgo) level=mkvgo ;;
                shell) break ;;
            esac ;;
        admin)
            _evey_paint_banner
            printf '  %sevey%s · ↑/↓ move · Enter options · Tab type/edit · q/Esc back%s\n' "$W" "$D" "$Z"
            for ((i=0; i<admin_count; i++)); do _evey_paint_admin_row "$i"; done
            _evey_navigate _evey_paint_admin_row "$admin_count" admin_sel

            if [ "$REPLY" = back ] || [ "$admin_sel" -eq $((admin_count-1)) ]; then level=main; continue; fi
            if [ "$REPLY" = edit ]; then _evey_prompt_command "${admin_prefills[admin_sel]}" || true; continue; fi
            option_verb="${admin_labels[admin_sel]}"
            if _evey_load_options "$option_verb"; then option_sel=0; level=options
            else _evey_prompt_command "${admin_prefills[admin_sel]}" || true; fi ;;
        mkvgo)
            _evey_paint_banner
            printf '  %smkvgo%s · ↑/↓ move · Enter fill the file · Tab edit · q/Esc back%s\n' "$W" "$D" "$Z"
            for ((i=0; i<mkvgo_count; i++)); do _evey_paint_mkvgo_row "$i"; done
            _evey_navigate _evey_paint_mkvgo_row "$mkvgo_count" mkvgo_sel

            if [ "$REPLY" = back ] || [ "$mkvgo_sel" -eq $((mkvgo_count-1)) ]; then level=main; continue; fi
            _evey_prompt_command "${mkvgo_prefills[mkvgo_sel]}" || true ;;   # run or cancel: stay here
        options)
            _evey_load_options "$option_verb"
            local option_count=${#option_labels[@]}
            printf '  %sevey ▸ %s%s · ↑/↓ move · Enter run/fill · Tab edit · q/Esc back%s\n' \
                "$W" "$option_verb" "$D" "$Z"
            for ((i=0; i<option_count; i++)); do _evey_paint_option_row "$i"; done
            _evey_navigate _evey_paint_option_row "$option_count" option_sel

            case "$REPLY" in
                back) level=admin ;;
                edit) _evey_prompt_command "${option_commands[option_sel]}" && level=admin ;;
                enter)
                    if [ "${option_kinds[option_sel]}" = run ]; then
                        history -s "${option_commands[option_sel]}"
                        _evey_run_command "${option_commands[option_sel]}"
                        level=admin
                    else
                        _evey_prompt_command "${option_commands[option_sel]}" && level=admin
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
