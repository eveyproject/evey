# bash completion for the `evey` CLI.
#
# With no subcommand `evey` boots the server; `evey admin <subcommand>` runs the
# recovery / maintenance tools instead. This gives an operator `evey <TAB>` for
# the admin group, its subcommands, their flags, and path/dir arguments.
_evey() {
    local cur prev sub opts
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    local subs="reset-password rotate-jwt-key restore-backup reindex-search migrate \
reclassify reindex-no-cue validate-streaming artwork-gc help"

    # `evey <TAB>` -> the admin group (the server itself takes no subcommand).
    if [ "$COMP_CWORD" -eq 1 ]; then
        COMPREPLY=( $(compgen -W "admin" -- "$cur") )
        return 0
    fi

    [ "${COMP_WORDS[1]}" = "admin" ] || return 0

    # `evey admin <TAB>` -> the subcommands.
    if [ "$COMP_CWORD" -eq 2 ]; then
        COMPREPLY=( $(compgen -W "$subs" -- "$cur") )
        return 0
    fi

    sub="${COMP_WORDS[2]}"

    # `evey admin migrate <TAB>` -> up/down/version.
    if [ "$sub" = "migrate" ] && [ "$COMP_CWORD" -eq 3 ]; then
        COMPREPLY=( $(compgen -W "up down version" -- "$cur") )
        return 0
    fi

    # A --root/--library value.
    case "$prev" in
        --root) COMPREPLY=( $(compgen -d -- "$cur") ); return 0 ;;
        --user|--library|--limit) return 0 ;;
    esac

    case "$sub" in
        reset-password)     opts="--user" ;;
        reindex-search)     opts="--library" ;;
        migrate)            opts="--no-backup" ;;
        reclassify)         opts="--all" ;;
        reindex-no-cue)     opts="--root --apply --limit --keep-backup" ;;
        validate-streaming) opts="--root --errors-only" ;;
        artwork-gc)         opts="--root --apply" ;;
        restore-backup)     COMPREPLY=( $(compgen -f -- "$cur") ); return 0 ;;
        *)                  opts="" ;;
    esac

    if [[ "$cur" == -* ]]; then
        COMPREPLY=( $(compgen -W "$opts" -- "$cur") )
        return 0
    fi
    COMPREPLY=( $(compgen -f -- "$cur") )
    return 0
}
complete -o filenames -F _evey evey
