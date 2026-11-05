#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# With a published digest the caller supplies an existing base; otherwise
# prepare the local base before the runtime build. No images are published.
if [ -z "${GENTOO_BASE_IMAGE:-}" ]; then
    docker build -f gentoo/Dockerfile.base \
        -t gentoo-update-intelligence/gentoo-base:local .
fi
base_image=${GENTOO_BASE_IMAGE:-gentoo-update-intelligence/gentoo-base:local}
docker run --rm --network none --entrypoint sh "$base_image" -ec '
    git --version
    python3 --version
    command -v egencache
    test -s /etc/ssl/certs/ca-certificates.crt
    if [ -e /lib/ld-musl-x86_64.so.1 ]; then
        sandbox=/usr/lib/libsandbox.so
    else
        sandbox=/usr/lib64/libsandbox.so
    fi
    LD_PRELOAD="$sandbox" /bin/bash -c true
    for tool in cp grep sed find tar awk; do
        "$tool" --version > /dev/null
    done
    ! command -v gcc
'
# Optional arguments are Compose global options, e.g. --env-file /dev/null.
docker compose "$@" build gentoo
