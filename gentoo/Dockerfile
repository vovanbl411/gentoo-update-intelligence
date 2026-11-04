FROM gentoo/stage3:latest

RUN emerge-webrsync && emerge --getbinpkg --oneshot dev-vcs/git

COPY scripts/sync-gentoo-repository.sh scripts/run-gentoo-sync.sh /usr/local/lib/gentoo-update-intelligence/
ENTRYPOINT ["sh", "-c", "exec sh /usr/local/lib/gentoo-update-intelligence/run-gentoo-sync.sh /metadata/gentoo.git \"$GENTOO_REPOSITORY_URL\""]
