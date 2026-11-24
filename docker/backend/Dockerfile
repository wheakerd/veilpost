# syntax=docker/dockerfile:1
FROM golang:1.27.4-alpine3.24 AS build
WORKDIR /app
COPY app/backend/go.mod app/backend/go.sum ./
RUN --mount=type=cache,id=veilpost-modules,target=/go/pkg/mod,from=gomodcache \
    GODEBUG=http2client=0 go mod download && go mod verify
COPY app/backend/ ./
RUN --mount=type=cache,id=veilpost-modules,target=/go/pkg/mod,from=gomodcache --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/veilpost-api ./cmd/api

FROM alpine:3.24
RUN apk add --no-cache ca-certificates && \
    addgroup -g 1000 app && adduser -D -u 1000 -G app app && \
    mkdir /data && chown app:app /data && chmod 700 /data
COPY --from=build /out/veilpost-api /usr/local/bin/veilpost-api
USER app
ENV VPN_HTTP_ADDR=0.0.0.0:8080 VPN_DATA_DIR=/data
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/veilpost-api"]
