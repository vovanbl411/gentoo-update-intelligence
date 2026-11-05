ARG GENTOO_BASE_IMAGE=gentoo-update-intelligence/gentoo-base:local

FROM golang:1.27-alpine@sha256:bff16a94c0b4b6d9642581e2a3aa81beb3edb940b2f5a3aac1d22c6a68cb7747 AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/metadata-worker ./cmd/metadata-worker

FROM ${GENTOO_BASE_IMAGE}

RUN git config --system --add safe.directory /metadata/gentoo.git \
    && git config --system --add safe.directory '/overlays/*'

COPY scripts/sync-gentoo-repository.sh scripts/run-gentoo-sync.sh /usr/local/lib/gentoo-update-intelligence/
COPY --from=build /out/metadata-worker /usr/local/bin/metadata-worker
RUN mkdir -p /metadata-cache /var/tmp/gentoo-update-intelligence \
    && chown portage:portage /metadata-cache /var/tmp/gentoo-update-intelligence
ENTRYPOINT ["sh", "-c", "exec sh /usr/local/lib/gentoo-update-intelligence/run-gentoo-sync.sh /metadata/gentoo.git \"$GENTOO_REPOSITORY_URL\""]
