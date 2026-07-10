# bash completion for the mkvgo CLI.
#
# mkvgo's command dispatch is a hand-rolled switch (no cobra), so it ships no
# completion of its own — this static script gives an admin `mkvgo <TAB>` for
# sub-commands, flag names, -target values, and file paths for arguments.
_mkvgo() {
    local cur prev cmds
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    cmds="info tracks chapters attachments tags probe keyframes to-vtt demux mux \
remove-track add-track edit edit-title edit-track extract-attachment add-attachment \
set-chapters extract-chapters remove-attachment extract-subtitle split join validate \
hash verify compare reindex reindex-inplace salvage serve serve-growing merge \
merge-subtitle edit-inplace to-mp4 from-mp4 to-webm to-hls hls-segment to-abr abr-segment \
watermark-segment concat-hls concat-segment extract-frame analyze fingerprint playability \
ladder ingest"

    # First token after `mkvgo` -> a sub-command (or the top-level flags).
    if [ "$COMP_CWORD" -eq 1 ]; then
        COMPREPLY=( $(compgen -W "$cmds --version --help" -- "$cur") )
        return 0
    fi

    # Value completion for known option that takes an argument.
    case "$prev" in
        -target)
            COMPREPLY=( $(compgen -W "safari chrome chromium-generic edge firefox \
brave opera vivaldi samsung-internet chromecast-gen3 mse-generic" -- "$cur") )
            return 0
            ;;
    esac

    # Option names.
    if [[ "$cur" == -* ]]; then
        COMPREPLY=( $(compgen -W "-json -f --force -analyze -reindex -deep-verify \
-replace -keep-backup -rollback -direct -auto -chapter-markers -sub-offset -target \
--aes-key --aes-key-uri --aes-rotate-segments --cenc-key --cenc-key-uri --variant --pattern" \
            -- "$cur") )
        return 0
    fi

    # Otherwise the argument is a media file/path.
    COMPREPLY=( $(compgen -f -- "$cur") )
    return 0
}
complete -o filenames -F _mkvgo mkvgo
