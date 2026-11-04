#!/bin/sh
set -eu

repository=${1:?usage: run-gentoo-sync.sh REPOSITORY SOURCE}
source_url=${2:?usage: run-gentoo-sync.sh REPOSITORY SOURCE}
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

while :; do
    if ! sh "$script_dir/sync-gentoo-repository.sh" "$repository" "$source_url"; then
        echo 'Gentoo repository synchronization failed; retaining any previously published commit' >&2
    fi
    sleep 10800
done
