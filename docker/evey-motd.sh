# Evey container: the interactive shell IS the Evey admin shell, with an arrow-key
# launcher on top. Sourced from /etc/bash.bashrc on an interactive shell.
#
#  - The prompt is branded `evey ▸`; it is a REAL bash shell, so native
#    `evey` / `mkvgo` Tab-completion works and any command / pipe / glob runs.
#  - Short admin verbs (load-test, reindex-search, migrate, ...) map 1:1 to
#    `evey admin <verb>` and complete like the real thing.
#  - The sub-menu (arrow keys) is a launcher: pick a command and it lands on the
#    prompt, prefilled and ready to Tab-complete / edit / run. You stay in the
#    REPL afterwards -- no auto-reopen. `menu` reopens it, `?` prints a static
#    cheat-sheet, `exit` leaves.
#  - No tty (piped / logs / non-interactive): a plain list, no key capture.
#  - Want a vanilla bash instead? `EVEY_SHELL=plain` (or `EVEY_MENU=off`) skips
#    all of it -- no branding, no launcher, no verbs. e.g.
#      docker exec -it -e EVEY_SHELL=plain evey-server bash

_EVEY_VERBS='reset-password reindex-search migrate validate-streaming reindex-no-cue artwork-gc load-test doctor rollback'

# _evey_palette sets colour vars (empty when not a tty / NO_COLOR).
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
    printf '  %sadmin shell%s   %s·%s   %stab-completion on%s\n\n' "$D" "$Z" "$D" "$Z" "$G" "$Z"
}

# The launcher rows: display label | prefill (lands on the prompt) | description.
# Kept in one place so the menu and the static cheat-sheet stay in sync.
_evey_rows() {
    local mkv; mkv=$(mkvgo --version 2>/dev/null | awk '{print $2}'); [ -n "$mkv" ] || mkv='?'
    lbl=(  'reset-password' 'reindex-search' 'migrate' 'validate-streaming' \
           'reindex-no-cue' 'artwork-gc' 'load-test' 'doctor' 'mkvgo <cmd>' 'close' )
    pre=(  'reset-password --user ' 'reindex-search' 'migrate ' 'validate-streaming --root ' \
           'reindex-no-cue --root ' 'artwork-gc' 'load-test --scenario balanced --max-viewers ' \
           'doctor' 'mkvgo ' '' )
    desc=( "reset a password (needs --user <email>)" "rebuild the search index" "up | down | version" \
           "report seek / HLS readiness (needs --root <dir>)" "repair MKV seek indexes (needs --root <dir>; --apply writes)" \
           "delete orphaned artwork (dry-run; --apply deletes)" \
           "ramp viewers, find the capacity ceiling" "health-check everything (--repair fixes safe stuff)" \
           "inspect / repair a media file  ($mkv)" \
           "type at the prompt (Tab completes)" )
}

# `?` -> a quick, non-modal reference.
evey_help() {
    local W R S D G Z; _evey_palette
    local -a lbl pre desc; _evey_rows
    _evey_banner
    local i
    for ((i=0; i<${#lbl[@]}-1; i++)); do
        printf '  %s%-20s%s %s%s%s\n' "$R" "${lbl[i]}" "$Z" "$D" "${desc[i]}" "$Z"
    done
    printf '\n  %sTab completes everywhere%s  ·  %smenu%s launcher  ·  %sexit%s leaves\n' \
        "$S" "$Z" "$R" "$Z" "$R" "$Z"
    printf '  %sfull form works too: %sevey admin <verb>%s%s\n\n' "$D" "$W" "$Z" "$Z"
}

# `menu` -> the arrow-key launcher. Pick a command; it lands on the prompt,
# prefilled and ready to Tab-complete / edit / run. Stay in the REPL after.
evey_menu() {
    local W R S D G Z; _evey_palette
    local -a lbl pre desc; _evey_rows
    _evey_banner

    local n=${#lbl[@]} close=$(( ${#lbl[@]} - 1 ))

    # No tty: static list, no key capture.
    if [ ! -t 0 ] || [ ! -t 1 ]; then
        local i
        for ((i=0; i<close; i++)); do
            printf '  %s▸%s %s%-20s%s %s%s%s\n' "$R" "$Z" "$W" "${lbl[i]}" "$Z" "$D" "${desc[i]}" "$Z"
        done
        printf '\n  %sTab-complete: %sevey admin%s %s<Tab>%s  ·  %smkvgo%s %s<Tab>%s\n\n' \
            "$D" "$R" "$Z" "$D" "$Z" "$R" "$Z" "$D" "$Z"
        return 0
    fi

    local sel=0 i key k2
    _row() {
        local i="$1" mark col
        if [ "$i" -eq "$sel" ]; then mark="${R}►${Z} "; col="$W"; else mark="  "; col="$D"; fi
        if [ "$i" -eq "$close" ]; then printf '%s%s⌨  %s%s\n' "$mark" "$col" "${desc[i]}" "$Z"
        else printf '%s%s%-20s%s %s%s%s\n' "$mark" "$col" "${lbl[i]}" "$Z" "$D" "${desc[i]}" "$Z"; fi
    }
    printf '  %s↑/↓ move · Enter pick · q close · %smenu%s%s reopens%s\n' "$D" "$R" "$Z" "$D" "$Z"
    for ((i=0; i<n; i++)); do _row "$i"; done

    while true; do
        IFS= read -rsn1 key 2>/dev/null || { sel="$close"; break; }
        if [ "$key" = $'\033' ]; then read -rsn2 -t 0.05 k2 2>/dev/null; key+="$k2"; fi
        case "$key" in
            $'\033[A'|$'\033OA'|k) sel=$(( (sel - 1 + n) % n )) ;;
            $'\033[B'|$'\033OB'|j|$'\t') sel=$(( (sel + 1) % n )) ;;
            ''|$'\n'|$'\r') break ;;
            q|Q|$'\033') sel="$close"; break ;;
            *) continue ;;
        esac
        printf '\033[%dA' "$n"
        for ((i=0; i<n; i++)); do printf '\033[2K'; _row "$i"; done
    done
    unset -f _row
    printf '\n'

    # `close` (or q): just drop to the prompt. A command: prefill it there so the
    # native completion can finish it. The DSR reply (\e[0n) triggers the bound
    # macro once the next prompt is drawn -> the text appears, editable.
    [ "$sel" -eq "$close" ] && return 0
    local pick="${pre[sel]}"
    [ -n "$pick" ] || return 0
    history -s "$pick"
    bind "\"\033[0n\": \"$pick\"" 2>/dev/null && printf '\033[5n'
}
menu() { evey_menu; }

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
        if [ -z "${EVEY_MOTD_SHOWN:-}" ]; then
            export EVEY_MOTD_SHOWN=1
            evey_menu
        fi
        ;;
esac
