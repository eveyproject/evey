# Evey container — one-time admin greeting, sourced from /etc/bash.bashrc on an
# interactive shell (e.g. `docker exec -it evey-server bash`). Prints the Evey
# wordmark and the two CLIs an operator has on hand — the `evey` maintenance
# commands and the `mkvgo` media tool — then gets out of the way. Colour is
# dropped when NO_COLOR is set or stdout is not a terminal.

case $- in
    *i*) : ;;                              # interactive only
    *) return 0 2>/dev/null || exit 0 ;;
esac
[ -n "${EVEY_MOTD_SHOWN:-}" ] && return 0 2>/dev/null
EVEY_MOTD_SHOWN=1
export EVEY_MOTD_SHOWN

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    W=$'\033[38;2;236;238;244m'; R=$'\033[38;2;220;38;38m'; S=$'\033[38;2;136;145;163m'
    D=$'\033[38;2;120;128;145m'; G=$'\033[38;2;108;192;139m'; Z=$'\033[0m'
else
    W=''; R=''; S=''; D=''; G=''; Z=''
fi

mkv=$(mkvgo --version 2>/dev/null | awk '{print $2}')
[ -n "$mkv" ] || mkv='?'

printf '\n'
printf '  %s███████╗%s██╗   ██╗%s███████╗%s██╗   ██╗%s\n' "$W" "$R" "$W" "$W" "$Z"
printf '  %s██╔════╝%s██║   ██║%s██╔════╝%s╚██╗ ██╔╝%s\n' "$W" "$R" "$W" "$W" "$Z"
printf '  %s█████╗  %s██║   ██║%s█████╗  %s ╚████╔╝ %s\n' "$W" "$R" "$W" "$W" "$Z"
printf '  %s██╔══╝  %s╚██╗ ██╔╝%s██╔══╝  %s  ╚██╔╝  %s\n' "$W" "$R" "$W" "$W" "$Z"
printf '  %s███████╗%s ╚████╔╝ %s███████╗%s   ██║   %s\n' "$W" "$R" "$W" "$W" "$Z"
printf '  %s╚══════╝%s  ╚═══╝  %s╚══════╝%s   ╚═╝   %s\n' "$W" "$R" "$W" "$W" "$Z"
printf '  %sadmin shell%s   %s·%s   %stab-completion on%s\n\n' "$D" "$Z" "$D" "$Z" "$G" "$Z"

printf '  %sevey admin%s %s<cmd>%s   %sserver maintenance & recovery%s\n' "$R" "$Z" "$W" "$Z" "$D" "$Z"
printf '     %sreset-password · migrate · reindex-search · validate-streaming · artwork-gc%s\n\n' "$S" "$Z"
printf '  %smkvgo%s %s<cmd>%s        %sinspect / repair media files%s   %s(%s)%s\n' "$R" "$Z" "$W" "$Z" "$D" "$Z" "$S" "$mkv" "$Z"
printf '     %singest -analyze · reindex-inplace · playability -target%s\n\n' "$S" "$Z"

printf '  %sTab-complete either.%s  %sevey admin help%s %sor%s %smkvgo%s %slists everything.%s\n\n' \
    "$D" "$Z" "$R" "$Z" "$D" "$Z" "$R" "$Z" "$D" "$Z"

unset W R S D G Z mkv
