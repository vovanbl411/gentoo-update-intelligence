FROM golang:1.27-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/metadata-worker ./cmd/metadata-worker

FROM gentoo/stage3:latest

RUN emerge-webrsync && emerge --getbinpkg --oneshot dev-vcs/git \
    && git config --system --add safe.directory /metadata/gentoo.git \
    && git config --system --add safe.directory '/overlays/*'

COPY scripts/sync-gentoo-repository.sh scripts/run-gentoo-sync.sh /usr/local/lib/gentoo-update-intelligence/
COPY --from=build /out/metadata-worker /usr/local/bin/metadata-worker
RUN mkdir -p /var/cache/gentoo-update-intelligence /var/tmp/gentoo-update-intelligence \
    && chown portage:portage /var/cache/gentoo-update-intelligence /var/tmp/gentoo-update-intelligence
ENTRYPOINT ["sh", "-c", "exec sh /usr/local/lib/gentoo-update-intelligence/run-gentoo-sync.sh /metadata/gentoo.git \"$GENTOO_REPOSITORY_URL\""]
