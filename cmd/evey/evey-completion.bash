# bash completion for the `evey` CLI - embedded in the binary: `evey completion bash`.
#
# With no subcommand `evey` boots the server; `evey admin <subcommand>` runs the
# recovery / maintenance tools. Media file arguments complete to media files and
# directories only; --root offers the library roots the server knows about.

# Media files (by extension) and directories matching the current word.
_evey_media_paths() {
    local f
    while IFS= read -r f; do
        [ -n "$f" ] || continue
        case "${f,,}" in
            *.mkv|*.mka|*.webm|*.mp4|*.m4v|*.mov|*.ts|*.m2ts|*.mts|*.avi|*.wmv|*.flv|*.ogv|*.mpg|*.mpeg|*.vob)
                COMPREPLY+=("$f") ;;
            *) [ -d "$f" ] && COMPREPLY+=("$f") ;;
        esac
    done < <(compgen -f -- "$1")
}

# The library roots (from the database) plus any directory.
_evey_root_paths() {
    COMPREPLY=( $(compgen -W "$(evey admin list-libraries 2>/dev/null)" -- "$1") $(compgen -d -- "$1") )
}

_evey() {
    local cur prev sub opts
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    local subs="reset-password rotate-jwt-key backup restore-backup reindex-search migrate \
reclassify reindex-no-cue validate-streaming artwork-gc load-test doctor rollback \
ladder-bench ladder-probe route-bench list-users list-libraries demo-seed help"

    if [ "$COMP_CWORD" -eq 1 ]; then
        COMPREPLY=( $(compgen -W "admin version help man completion" -- "$cur") )
        return 0
    fi

    case "${COMP_WORDS[1]}" in
        completion) [ "$COMP_CWORD" -eq 2 ] && COMPREPLY=( $(compgen -W "bash" -- "$cur") ); return 0 ;;
        man)        [ "$COMP_CWORD" -eq 2 ] && COMPREPLY=( $(compgen -W "--text" -- "$cur") ); return 0 ;;
        admin)      ;;
        *)          return 0 ;;
    esac

    if [ "$COMP_CWORD" -eq 2 ]; then
        COMPREPLY=( $(compgen -W "$subs" -- "$cur") )
        return 0
    fi

    sub="${COMP_WORDS[2]}"

    if [ "$COMP_CWORD" -eq 3 ]; then
        case "$sub" in
            migrate)   COMPREPLY=( $(compgen -W "up down version" -- "$cur") ); return 0 ;;
            load-test) [[ "$cur" != -* ]] && { COMPREPLY=( $(compgen -W "minimal standard max" -- "$cur") ); return 0; } ;;
        esac
    fi

    # Values of the flags that take one.
    case "$prev" in
        --root)     _evey_root_paths "$cur"; return 0 ;;
        --user)     COMPREPLY=( $(compgen -W "$(evey admin list-users 2>/dev/null)" -- "$cur") ); return 0 ;;
        --scenario) COMPREPLY=( $(compgen -W "transcode natural mkvgo light balanced heavy" -- "$cur") ); return 0 ;;
        --backend)  COMPREPLY=( $(compgen -W "nvenc vaapi qsv none" -- "$cur") ); return 0 ;;
        --mode)
            case "$sub" in
                reindex-no-cue) opts="auto inplace rewrite" ;;
                load-test)      opts="ramp flash seek endurance" ;;
                ladder-bench)   opts="full fast incremental" ;;
            esac
            COMPREPLY=( $(compgen -W "$opts" -- "$cur") ); return 0 ;;
        --cache|--workdir)          COMPREPLY=( $(compgen -d -- "$cur") ); return 0 ;;
        --out|--json|--capability)  COMPREPLY=( $(compgen -f -- "$cur") ); return 0 ;;
        --library|--limit|--max-viewers|--duration|--scenes|--window|--heights|--crf|--fps|\
        --preset|--ref-height|--ref-crf|--encoders|--routes|--classes|--device|--keep-backup)
            return 0 ;;
    esac

    case "$sub" in
        reset-password)     opts="--user" ;;
        reindex-search)     opts="--library --purge" ;;
        migrate)            opts="--no-backup" ;;
        reclassify)         opts="--all" ;;
        reindex-no-cue)     opts="--root --apply --limit --mode --deep-verify --resync --keep-backup" ;;
        validate-streaming) opts="--root --errors-only" ;;
        artwork-gc)         opts="--root --apply" ;;
        demo-seed)          opts="--apply --media" ;;
        load-test)          opts="--scenario --mode --max-viewers --duration --cold-cache" ;;
        doctor)             opts="--repair" ;;
        ladder-bench)       opts="--scenes --window --heights --crf --preset --out --ref-height --ref-crf --efficiency --mode --cache --encoders --device" ;;
        ladder-probe)       opts="--window --cache --out" ;;
        route-bench)        opts="--backend --device --heights --fps --routes --classes --duration --workdir --capability --out --json" ;;
        backup)            opts="--include-telemetry" ;;
        restore-backup)
            COMPREPLY=( $(compgen -d -- "$cur") $(compgen -f -X '!*.sql.gz' -- "$cur") ); return 0 ;;
        *)                  opts="" ;;
    esac

    if [[ "$cur" == -* ]]; then
        COMPREPLY=( $(compgen -W "$opts" -- "$cur") )
        return 0
    fi
    case "$sub" in
        ladder-bench|ladder-probe|route-bench) _evey_media_paths "$cur" ;;
        *) COMPREPLY=( $(compgen -f -- "$cur") ) ;;
    esac
    return 0
}
complete -o filenames -F _evey evey
