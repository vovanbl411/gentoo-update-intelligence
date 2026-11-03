#!/bin/sh
set -eu

export CODEGRAPH_TELEMETRY=0
export DO_NOT_TRACK=1
export CODEGRAPH_NO_DOWNLOAD=1
export CODEGRAPH_NO_DAEMON=1

cd "$(dirname "$0")/.."

if command -v codegraph >/dev/null 2>&1; then
    codegraph_bin=$(command -v codegraph)
elif [ -x "$HOME/.npm-global/bin/codegraph" ]; then
    codegraph_bin="$HOME/.npm-global/bin/codegraph"
else
    echo "CodeGraph is not installed or is not executable." >&2
    exit 127
fi

case "${1-}" in
    explore|query|context|node|files|callers|callees|impact|affected)
        if ! "$codegraph_bin" sync --quiet >&2; then
            echo "CodeGraph sync failed; query cancelled. Check the index with: sh scripts/codegraph.sh status" >&2
            exit 1
        fi
        ;;
esac

exec "$codegraph_bin" "$@"
