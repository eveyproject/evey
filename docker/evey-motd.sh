# Evey container greeting + a reusable `menu` command. Sourced from
# /etc/bash.bashrc on an interactive shell (`docker exec -it evey-server bash`):
# it defines evey_menu()/menu() and shows the menu once per shell. Run `menu`
# any time to bring the assisted picker back. Arrow keys move; Enter drops the
# chosen command onto the real prompt (Tab-completion applies); the last entry
# gives a blank prompt to type your own. No tty (piped/logs): a plain list.

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

    local -a cmd desc
    cmd=( 'evey admin reset-password' 'evey admin migrate up' 'evey admin reindex-search' \
          'evey admin validate-streaming' 'evey admin reindex-no-cue' 'evey admin artwork-gc' \
          'mkvgo <cmd>' '' )
    desc=( "reset a user's password" "apply pending migrations" "rebuild the search index" \
           "report seek / HLS readiness" "repair MKV seek indexes" "delete orphaned artwork" \
           "inspect / repair a media file  ($mkv)" "type your own  —  evey / mkvgo + Tab Tab" )
    local n=${#cmd[@]} last=$(( ${#cmd[@]} - 1 ))

    if [ ! -t 0 ] || [ ! -t 1 ]; then
        local i
        for ((i=0; i<last; i++)); do
            printf '  %s▸%s %s%-31s%s %s%s%s\n' "$R" "$Z" "$W" "${cmd[i]}" "$Z" "$D" "${desc[i]}" "$Z"
        done
        printf '\n  %sTab-complete: %sevey admin%s %s<Tab>%s  ·  %smkvgo%s %s<Tab>%s\n\n' \
            "$D" "$R" "$Z" "$D" "$Z" "$R" "$Z" "$D" "$Z"
        return 0
    fi

    local sel=0 i key k2
    _row() {
        local i="$1" mark col
        if [ "$i" -eq "$sel" ]; then mark="${R}►${Z} "; col="$W"; else mark="  "; col="$D"; fi
        if [ "$i" -eq "$last" ]; then
            printf '%s%s⌨  %s%s\n' "$mark" "$col" "${desc[i]}" "$Z"
        else
            printf '%s%s%-31s%s %s%s%s\n' "$mark" "$col" "${cmd[i]}" "$Z" "$D" "${desc[i]}" "$Z"
        fi
    }
    printf '  %s↑/↓ choose · Enter load · q for a blank prompt · run %smenu%s%s to reopen%s\n' \
        "$D" "$R" "$Z" "$D" "$Z"
    for ((i=0; i<n; i++)); do _row "$i"; done

    while true; do
        IFS= read -rsn1 key 2>/dev/null || { sel=-1; break; }
        if [ "$key" = $'\033' ]; then read -rsn2 -t 0.05 k2 2>/dev/null; key+="$k2"; fi
        case "$key" in
            $'\033[A'|$'\033OA'|k) sel=$(( (sel - 1 + n) % n )) ;;
            $'\033[B'|$'\033OB'|j|$'\t') sel=$(( (sel + 1) % n )) ;;
            ''|$'\n'|$'\r') break ;;
            q|Q|$'\033') sel=-1; break ;;
            *) continue ;;
        esac
        printf '\033[%dA' "$n"
        for ((i=0; i<n; i++)); do printf '\033[2K'; _row "$i"; done
    done
    unset -f _row

    printf '\n'
    if [ "$sel" -ge 0 ] && [ "$sel" -lt "$last" ]; then
        local pick="${cmd[sel]} "
        history -s "${cmd[sel]}"
        bind "\"\033[0n\": \"$pick\"" 2>/dev/null && printf '\033[5n'
    fi
}

menu() { evey_menu; }

# Auto-show once per interactive shell.
case $- in
    *i*)
        if [ -z "${EVEY_MOTD_SHOWN:-}" ]; then
            EVEY_MOTD_SHOWN=1
            export EVEY_MOTD_SHOWN
            evey_menu
        fi
        ;;
esac
