#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
export OPENSPEC_TELEMETRY=0
export DO_NOT_TRACK=1
export XDG_CONFIG_HOME="$PWD/tools/openspec/.config"
export XDG_DATA_HOME="$PWD/tools/openspec/.data"

openspec_bin="$PWD/tools/openspec/node_modules/.bin/openspec"
if [ ! -x "$openspec_bin" ]; then
    echo "Install OpenSpec with: npm ci --prefix tools/openspec --ignore-scripts --no-audit --no-fund" >&2
    exit 127
fi
exec "$openspec_bin" "$@"
