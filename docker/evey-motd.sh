# Evey container: an assisted command menu that stays your home base. Sourced
# from /etc/bash.bashrc on an interactive shell. Pick a command → you land on the
# REAL prompt (so `evey admin <Tab>` / `mkvgo <Tab>` completion works), prefilled
# with a completable prefix; when the command finishes you come back to the menu.
# You only leave when you choose "leave" (or press q). `menu` reopens it anytime.
# No tty (piped/logs): a plain list, no key capture.

evey_menu() {
    local compact="${1:-}"
    local W R S D G Z mkv
    if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
        W=$'\033[38;2;236;238;244m'; R=$'\033[38;2;220;38;38m'; S=$'\033[38;2;136;145;163m'
        D=$'\033[38;2;120;128;145m'; G=$'\033[38;2;108;192;139m'; Z=$'\033[0m'
    else
        W=''; R=''; S=''; D=''; G=''; Z=''
    fi
    mkv=$(mkvgo --version 2>/dev/null | awk '{print $2}'); [ -n "$mkv" ] || mkv='?'

    printf '\n'
    if [ "$compact" != compact ]; then
        printf '  %s███████╗%s██╗   ██╗%s███████╗%s██╗   ██╗%s\n' "$W" "$R" "$W" "$W" "$Z"
        printf '  %s██╔════╝%s██║   ██║%s██╔════╝%s╚██╗ ██╔╝%s\n' "$W" "$R" "$W" "$W" "$Z"
        printf '  %s█████╗  %s██║   ██║%s█████╗  %s ╚████╔╝ %s\n' "$W" "$R" "$W" "$W" "$Z"
        printf '  %s██╔══╝  %s╚██╗ ██╔╝%s██╔══╝  %s  ╚██╔╝  %s\n' "$W" "$R" "$W" "$W" "$Z"
        printf '  %s███████╗%s ╚████╔╝ %s███████╗%s   ██║   %s\n' "$W" "$R" "$W" "$W" "$Z"
        printf '  %s╚══════╝%s  ╚═══╝  %s╚══════╝%s   ╚═╝   %s\n' "$W" "$R" "$W" "$W" "$Z"
        printf '  %sadmin shell%s   %s·%s   %stab-completion on%s\n\n' "$D" "$Z" "$D" "$Z" "$G" "$Z"
    fi

    # display | prefill (what lands on the prompt, ending where Tab helps) | desc
    local -a lbl pre desc
    lbl=(  'evey admin reset-password' 'evey admin migrate' 'evey admin reindex-search' \
           'evey admin validate-streaming' 'evey admin reindex-no-cue' 'evey admin artwork-gc' \
           'evey admin load-test' 'mkvgo <cmd>' 'type your own' 'leave' )
    pre=(  'evey admin reset-password --user ' 'evey admin migrate ' 'evey admin reindex-search' \
           'evey admin validate-streaming --root ' 'evey admin reindex-no-cue --root ' 'evey admin artwork-gc' \
           'evey admin load-test --scenario balanced --max-viewers ' 'mkvgo ' '' '' )
    desc=( "reset a user's password" "up | down | version" "rebuild the search index" \
           "report seek / HLS readiness" "repair MKV seek indexes" "delete orphaned artwork" \
           "ramp viewers, find the capacity ceiling" "inspect / repair a media file  ($mkv)" \
           "empty prompt — evey / mkvgo + Tab Tab" "drop to a plain shell" )
    local n=${#lbl[@]} typ=$(( ${#lbl[@]} - 2 )) leave=$(( ${#lbl[@]} - 1 ))

    if [ ! -t 0 ] || [ ! -t 1 ]; then
        local i
        for ((i=0; i<typ; i++)); do
            printf '  %s▸%s %s%-31s%s %s%s%s\n' "$R" "$Z" "$W" "${lbl[i]}" "$Z" "$D" "${desc[i]}" "$Z"
        done
        printf '\n  %sTab-complete: %sevey admin%s %s<Tab>%s  ·  %smkvgo%s %s<Tab>%s\n\n' \
            "$D" "$R" "$Z" "$D" "$Z" "$R" "$Z" "$D" "$Z"
        return 0
    fi

    local sel=0 i key k2
    _row() {
        local i="$1" mark col
        if [ "$i" -eq "$sel" ]; then mark="${R}►${Z} "; col="$W"; else mark="  "; col="$D"; fi
        if [ "$i" -eq "$typ" ]; then   printf '%s%s⌨  %s%s\n' "$mark" "$col" "${desc[i]}" "$Z"
        elif [ "$i" -eq "$leave" ]; then printf '%s%s⏻  %s%s\n' "$mark" "$col" "${desc[i]}" "$Z"
        else printf '%s%s%-31s%s %s%s%s\n' "$mark" "$col" "${lbl[i]}" "$Z" "$D" "${desc[i]}" "$Z"; fi
    }
    printf '  %s↑/↓ choose · Enter run · q leave · %smenu%s%s reopens%s\n' "$D" "$R" "$Z" "$D" "$Z"
    for ((i=0; i<n; i++)); do _row "$i"; done

    while true; do
        IFS= read -rsn1 key 2>/dev/null || { sel="$leave"; break; }
        if [ "$key" = $'\033' ]; then read -rsn2 -t 0.05 k2 2>/dev/null; key+="$k2"; fi
        case "$key" in
            $'\033[A'|$'\033OA'|k) sel=$(( (sel - 1 + n) % n )) ;;
            $'\033[B'|$'\033OB'|j|$'\t') sel=$(( (sel + 1) % n )) ;;
            ''|$'\n'|$'\r') break ;;
            q|Q|$'\033') sel="$leave"; break ;;
            *) continue ;;
        esac
        printf '\033[%dA' "$n"
        for ((i=0; i<n; i++)); do printf '\033[2K'; _row "$i"; done
    done
    unset -f _row
    printf '\n'

    if [ "$sel" -eq "$leave" ]; then
        return 0                                     # plain shell; no reopen
    fi
    # A command (or the free-type entry): come back to the menu after it runs.
    _EVEY_MENU_PENDING=1
    local pick="${pre[sel]}"
    if [ -n "$pick" ]; then
        history -s "$pick"
        printf '  %s→%s %s%s%s%s   (Tab to complete)%s\n' "$R" "$Z" "$W" "$pick" "$Z" "$D" "$Z"
        bind "\"\033[0n\": \"$pick\"" 2>/dev/null && printf '\033[5n'
    fi
}

menu() { evey_menu; }

# PROMPT_COMMAND hook: two-phase so the prefilled prompt shows once, then the
# menu reopens after that command finishes.
__evey_prompt() {
    case "${_EVEY_MENU_PENDING:-}" in
        1) _EVEY_MENU_PENDING=2 ;;
        2) unset _EVEY_MENU_PENDING; evey_menu compact ;;
    esac
}

case $- in
    *i*)
        case ";${PROMPT_COMMAND:-};" in
            *";__evey_prompt;"*) : ;;
            *) PROMPT_COMMAND="${PROMPT_COMMAND:+$PROMPT_COMMAND;}__evey_prompt" ;;
        esac
        if [ -z "${EVEY_MOTD_SHOWN:-}" ]; then
            EVEY_MOTD_SHOWN=1
            export EVEY_MOTD_SHOWN
            evey_menu
        fi
        ;;
esac
