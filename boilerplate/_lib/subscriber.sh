# Helpers and variables for subscriber automation
#
# Source this file from subscriber[-*].
#
# If your command has subcommands, define SUBCOMMANDS as a map of
# [subcmd_name]='Description one-liner' *before* sourcing this library
# and it will parse the command line up to that point for you, setting
# the SUBCOMMAND variable and leaving everything else in $@. No explicit
# usage function is necessary.
#
# Otherwise, define your usage() function *before* sourcing this library
# and it will handle variants of [-[-]]h[elp] for you.

CMD=${SOURCER##*/}

_subcommand_usage() {
    echo "Usage: $CMD SUBCOMMAND ..."
    for subcommand in "${!SUBCOMMANDS[@]}"; do
        echo
        echo "==========="
        echo "$CMD $subcommand"
        echo "    ${SUBCOMMANDS[$subcommand]}"
    done
    exit -1
}

# Regex for help, -h, -help, --help, etc.
# NOTE: This will match a raw 'h'. That's probably okay, since if
# there's a conflict, 'h' would be ambiguous anyway.
_helpre='^-*h(elp)?$'

# Subcommand processing
if [[ ${#SUBCOMMANDS[@]} -ne 0 ]]; then

    # No subcommand specified
    [[ $# -eq 0 ]] && _subcommand_usage

    subcmd=$1
    shift

    [[ "$subcmd" =~ $_helpre ]] && _subcommand_usage

    # Allow unique prefixes
    SUBCOMMAND=
    for key in "${!SUBCOMMANDS[@]}"; do
        if [[ $key == "$subcmd"* ]]; then
            # If SUBCOMMAND is already set, this is an ambiguous prefix.
            if [[ -n "$SUBCOMMAND" ]]; then
                err "Ambiguous subcommand prefix: '$subcmd' matches (at least): ['$SUBCOMMAND', '$key']"
            fi
            SUBCOMMAND=$key
        fi
    done
    [[ -n "$SUBCOMMAND" ]] || err "Unknown subcommand '$subcmd'. Try 'help' for usage."

    # We got a valid, unique subcommand. Run the helper with the remaining CLI args.
    exec $HERE/$CMD-$SUBCOMMAND "$@"
fi

[[ "$1" =~ $_helpre ]] && usage

SUBSCRIBERS_FILE=$REPO_ROOT/subscribers.yaml

## subscriber_list FILTER
#
# Prints a list of subscribers registered in the $SUBSCRIBERS_FILE.
#
# FILTER:
#       all:        Prints all subscribers
#       onboarded:  Prints only onboarded subscribers
subscriber_list() {
    local filt
    case $1 in
        all) filt='[*]';;
        # TODO: Right now subscribers are only "manual".
        onboarded) filt='(conventions.**.status==manual)';;
    esac
    yq r $SUBSCRIBERS_FILE "subscribers${filt}.name"
}

## last_bp_commit ORG/PROJ
#
# Prints the commit hash of the specified repository's boilerplate
# level, or the empty string if the repository is not onboarded.
#
# ORG/PROJ: github organization and project name, e.g.
#           "openshift/my-wizbang-operator".
last_bp_commit() {
    local repo=$1
    local lbc
    for default_branch in master main; do
        lbc=$(curl -s https://raw.githubusercontent.com/$repo/$default_branch/boilerplate/_data/last-boilerplate-commit)
        if [[ "$lbc" != "404: Not Found" ]]; then
            echo $lbc | cut -c 1-7
            return
        fi
    done
}

## commits_behind_bp_master HASH
#
# Prints how many merge commits behind boilerplate master HASH is. If
# HASH is empty/unspecified, prints the total number of merge commits in
# the boilerplate repo.
commits_behind_bp_master() {
    local hash=$1
    local range=master
    if [[ -n "$hash" ]]; then
        range=$hash..master
    fi
    git rev-list --count --merges $range
}

