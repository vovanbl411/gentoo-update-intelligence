#!/bin/sh
# Publish only a completely fetched and verified commit. Existing objects and
# published commits are retained for analyses that have already pinned them.
set -eu

repository=${1:?usage: sync-gentoo-repository.sh REPOSITORY SOURCE}
source_url=${2:?usage: sync-gentoo-repository.sh REPOSITORY SOURCE}

if [ ! -d "$repository" ]; then
    git init --bare "$repository" >/dev/null
fi

# A private ref protects the candidate from garbage collection while it is
# verified. Disable automatic maintenance so previously pinned commits remain.
git -C "$repository" -c gc.auto=0 -c maintenance.auto=false fetch --no-tags --force "$source_url" '+HEAD:refs/gentoo/candidate'
commit=$(git -C "$repository" rev-parse --verify 'refs/gentoo/candidate^{commit}')
git -C "$repository" fsck --connectivity-only --no-reflogs "$commit" >/dev/null
# Retain every published commit independently of the moving published ref.
git -C "$repository" update-ref "refs/gentoo/snapshots/$commit" "$commit"
git -C "$repository" update-ref refs/gentoo/published "$commit"
