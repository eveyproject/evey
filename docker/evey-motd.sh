# Evey container — one-time admin greeting, sourced from /etc/bash.bashrc on an
# interactive shell (e.g. `docker exec -it evey-server bash`). Prints the Evey
# wordmark and a few mkvgo examples, then gets out of the way. Colour is dropped
# when NO_COLOR is set or stdout is not a terminal.

case $- in
    *i*) : ;;                              # interactive only
    *) return 0 2>/dev/null || exit 0 ;;
esac
[ -n "${EVEY_MOTD_SHOWN:-}" ] && return 0 2>/dev/null
EVEY_MOTD_SHOWN=1
export EVEY_MOTD_SHOWN

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    W=$'\033[38;2;236;238;244m'; R=$'\033[38;2;220;38;38m'; S=$'\033[38;2;136;145;163m'
    D=$'\033[38;2;120;128;145m'; G=$'\033[38;2;108;192;139m'; C=$'\033[38;2;99;179;201m'; Z=$'\033[0m'
else
    W=''; R=''; S=''; D=''; G=''; C=''; Z=''
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
printf '  %sadmin shell%s  %s·%s  %smkvgo%s %s%s%s %s·%s %stab-completion on%s\n\n' \
    "$D" "$Z" "$D" "$Z" "$S" "$Z" "$S" "$mkv" "$Z" "$D" "$Z" "$G" "$Z"
printf '  %sInspect & repair media without leaving the box:%s\n\n' "$D" "$Z"
printf '    %s$%s %smkvgo%s ingest %s-analyze -json%s %s/media/movie.mkv%s\n' "$G" "$Z" "$R" "$Z" "$C" "$Z" "$D" "$Z"
printf '    %s$%s %smkvgo%s reindex-inplace       %s/media/movie.mkv%s\n' "$G" "$Z" "$R" "$Z" "$D" "$Z"
printf '    %s$%s %smkvgo%s playability %s-target%s chromecast-gen3 %sfile.mkv%s\n\n' "$G" "$Z" "$R" "$Z" "$C" "$Z" "$D" "$Z"
printf '  %sType%s %smkvgo%s %s<TAB><TAB>%s %sto list all commands.%s\n\n' "$D" "$Z" "$R" "$Z" "$S" "$Z" "$D" "$Z"

unset W R S D G C Z mkv
