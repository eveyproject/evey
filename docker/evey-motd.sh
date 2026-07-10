# Evey container: the interactive shell IS the Evey admin shell. Sourced from
# /etc/bash.bashrc on an interactive shell. It brands the bash prompt (`evey ▸`),
# keeps NATIVE `evey` / `mkvgo` Tab-completion (this is the real prompt, so it
# just works), adds short aliases for the admin tools (`load-test` runs
# `evey admin load-test`), and prints a cheat-sheet once. It stays a real shell:
# any command runs, pipes / globs / history all work. `menu` reprints the sheet,
# `exit` leaves. No tty (piped / logs / non-interactive): nothing is emitted and
# the prompt is left untouched.

# The short admin verbs, mapped 1:1 to `evey admin <verb>`.
_EVEY_VERBS='reset-password reindex-search migrate validate-streaming reindex-no-cue artwork-gc load-test'

# evey_menu prints the banner + command cheat-sheet. Colours only on a tty.
evey_menu() {
    local W R S D G Z mkv
    if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
        W=$'\033[38;2;236;238;244m'; R=$'\033[38;2;220;38;38m'; S=$'\033[38;2;136;145;163m'
        D=$'\033[38;2;120;128;145m'; G=$'\033[38;2;108;192;139m'; Z=$'\033[0m'
    else
        W=''; R=''; S=''; D=''; G=''; Z=''
    fi
    mkv=$(mkvgo --version 2>/dev/null | awk '{print $2}'); [ -n "$mkv" ] || mkv='?'

    printf '\n'
    printf '  %s███████╗%s██╗   ██╗%s███████╗%s██╗   ██╗%s\n' "$W" "$R" "$W" "$W" "$Z"
    printf '  %s██╔════╝%s██║   ██║%s██╔════╝%s╚██╗ ██╔╝%s\n' "$W" "$R" "$W" "$W" "$Z"
    printf '  %s█████╗  %s██║   ██║%s█████╗  %s ╚████╔╝ %s\n' "$W" "$R" "$W" "$W" "$Z"
    printf '  %s██╔══╝  %s╚██╗ ██╔╝%s██╔══╝  %s  ╚██╔╝  %s\n' "$W" "$R" "$W" "$W" "$Z"
    printf '  %s███████╗%s ╚████╔╝ %s███████╗%s   ██║   %s\n' "$W" "$R" "$W" "$W" "$Z"
    printf '  %s╚══════╝%s  ╚═══╝  %s╚══════╝%s   ╚═╝   %s\n' "$W" "$R" "$W" "$W" "$Z"
    printf '  %sadmin shell%s   %s·%s   %stab-completion on%s\n\n' "$D" "$Z" "$D" "$Z" "$G" "$Z"

    # verb | description  (verbs are the short aliases; `evey admin <verb>` also works)
    local -a v desc
    v=(    'reset-password' 'reindex-search' 'migrate' 'validate-streaming' \
           'reindex-no-cue' 'artwork-gc' 'load-test' )
    desc=( "reset a user's password" "rebuild the search index" "up | down | version" \
           "report seek / HLS readiness" "repair MKV seek indexes" "delete orphaned artwork" \
           "ramp viewers, find the capacity ceiling" )
    local i
    for ((i=0; i<${#v[@]}; i++)); do
        printf '  %s%-20s%s %s%s%s\n' "$R" "${v[i]}" "$Z" "$D" "${desc[i]}" "$Z"
    done
    printf '  %s%-20s%s %s%s%s\n' "$R" "mkvgo <cmd>" "$Z" "$D" "inspect / repair a media file  ($mkv)" "$Z"

    printf '\n  %sTab completes everywhere%s  ·  %smenu%s reprints this  ·  %sexit%s leaves\n' \
        "$S" "$Z" "$R" "$Z" "$R" "$Z"
    printf '  %sfull form works too: %sevey admin <verb>%s%s\n\n' "$D" "$W" "$Z" "$Z"
}
menu() { evey_menu; }

case $- in
    *i*)
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

        # `?` is a handy synonym for the cheat-sheet (aliases only expand here).
        alias '?'='evey_menu' 2>/dev/null

        # Brand the prompt.
        if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
            PS1='\[\033[38;2;220;38;38m\]evey\[\033[0m\] \[\033[38;2;136;145;163m\]▸\[\033[0m\] '
        else
            PS1='evey ▸ '
        fi

        # Greet once per shell.
        if [ -z "${EVEY_MOTD_SHOWN:-}" ]; then
            export EVEY_MOTD_SHOWN=1
            evey_menu
        fi
        ;;
esac
