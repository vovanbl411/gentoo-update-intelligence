#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
work=$(mktemp -d)

git init -q "$work/source"
git -C "$work/source" config user.name Test
git -C "$work/source" config user.email test@example.invalid
printf 'first\n' > "$work/source/file"
git -C "$work/source" add file
git -C "$work/source" -c commit.gpgsign=false commit -qm first
first=$(git -C "$work/source" rev-parse HEAD)

sh "$root/scripts/sync-gentoo-repository.sh" "$work/mirror" "$work/source"
test "$(git -C "$work/mirror" rev-parse refs/gentoo/published)" = "$first"
test "$(git -C "$work/mirror" rev-parse "refs/gentoo/snapshots/$first")" = "$first"

if sh "$root/scripts/sync-gentoo-repository.sh" "$work/mirror" "$work/missing"; then
    echo 'expected fetch to fail' >&2
    exit 1
fi
test "$(git -C "$work/mirror" rev-parse refs/gentoo/published)" = "$first"

printf 'second\n' > "$work/source/file"
git -C "$work/source" -c commit.gpgsign=false commit -qam second
second=$(git -C "$work/source" rev-parse HEAD)
sh "$root/scripts/sync-gentoo-repository.sh" "$work/mirror" "$work/source"
test "$(git -C "$work/mirror" rev-parse refs/gentoo/published)" = "$second"
test "$(git -C "$work/mirror" rev-parse "refs/gentoo/snapshots/$first")" = "$first"
test "$(git -C "$work/mirror" rev-parse "refs/gentoo/snapshots/$second")" = "$second"

if sh "$root/scripts/sync-gentoo-repository.sh" "$work/empty" "$work/missing"; then
    echo 'expected initial fetch to fail' >&2
    exit 1
fi
if git -C "$work/empty" show-ref --verify --quiet refs/gentoo/published; then
    echo 'failed initial fetch published a ref' >&2
    exit 1
fi

# Stop the scheduler at its first sleep and inspect the requested interval.
mkdir "$work/bin"
cat > "$work/bin/sleep" <<'EOF'
#!/bin/sh
printf '%s\n' "$1" > "$SYNC_TEST_INTERVAL"
exit 1
EOF
chmod +x "$work/bin/sleep"
if PATH="$work/bin:$PATH" SYNC_TEST_INTERVAL="$work/interval" sh "$root/scripts/run-gentoo-sync.sh" "$work/mirror" "$work/missing"; then
    echo 'expected the test sleep to stop the scheduler' >&2
    exit 1
fi
test "$(cat "$work/interval")" = 10800
test "$(git -C "$work/mirror" rev-parse refs/gentoo/published)" = "$second"
echo 'repository synchronization tests passed'
