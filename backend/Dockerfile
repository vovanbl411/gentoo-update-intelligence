FROM golang:1.27-alpine AS build

WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gentoo-update-intelligence-server ./cmd/gentoo-update-intelligence-server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates git \
    && git config --system --add safe.directory /metadata/gentoo.git \
    && addgroup -S app && adduser -S -G app app \
    && mkdir /overlays && chown app:app /overlays
COPY --from=build /out/gentoo-update-intelligence-server /usr/local/bin/gentoo-update-intelligence-server

USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/gentoo-update-intelligence-server"]
