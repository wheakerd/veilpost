# syntax=docker.io/docker/dockerfile:1.29.0@sha256:572cebbc2b15e6f84c077742952674889cb63cd5e9b80abdffcbe587b6f19183

# AmneziaWG server image built from pinned upstream sources.
#
# Each source archive is fetched by full commit and checked against its
# SHA-256 before use. To move to another upstream release, change the version,
# commit, and checksum together (see README.md).

ARG AWG_GO_VERSION=v3.1.20261116
ARG AWG_GO_COMMIT=08fe3d33bbb116be09a6183d02609311b8b0c9e8
ARG AWG_GO_SHA256=e33d8a62928222592df23b75356d79e0ea6b590943cbe3a4cfcdf9c357bb416e
ARG AWG_TOOLS_VERSION=v3.1.20260812
ARG AWG_TOOLS_COMMIT=ee0f0a9aa34ff0a0da4b3433b9512781cfe02843
ARG AWG_TOOLS_SHA256=22438f231d39ea27e4bdc69707ec3103357f3cecc9ad3eecba18a61e5b39c9b9

FROM docker.io/library/golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS go-builder

ARG AWG_GO_VERSION
ARG AWG_GO_COMMIT
ARG AWG_GO_SHA256

ENV GOTOOLCHAIN=local \
    GOFLAGS=-mod=readonly

# A C toolchain is needed only for the tests: the upstream "outline" package
# (a library the daemon does not import) depends on a cgo-only resolver.
RUN apk add --no-cache build-base

ADD --checksum=sha256:${AWG_GO_SHA256} \
    https://github.com/amnezia-vpn/amneziawg-go/archive/${AWG_GO_COMMIT}.tar.gz \
    /tmp/amneziawg-go.tar.gz

WORKDIR /src/amneziawg-go
RUN tar -xzf /tmp/amneziawg-go.tar.gz --strip-components=1 \
    && printf 'package main\n\nconst Version = "%s"\n' "${AWG_GO_VERSION}" >version.go \
    && go mod download \
    && go mod verify \
    && if ! CGO_ENABLED=1 go test ./... >/tmp/amneziawg-go-test.log 2>&1; then tail -n 200 /tmp/amneziawg-go-test.log; exit 1; fi \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -buildid=' -o /out/amneziawg-go . \
    && /out/amneziawg-go --version

FROM docker.io/library/alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf AS tools-builder

ARG AWG_TOOLS_COMMIT
ARG AWG_TOOLS_SHA256

RUN apk add --no-cache build-base linux-headers

ADD --checksum=sha256:${AWG_TOOLS_SHA256} \
    https://github.com/amnezia-vpn/amneziawg-tools/archive/${AWG_TOOLS_COMMIT}.tar.gz \
    /tmp/amneziawg-tools.tar.gz

WORKDIR /src/amneziawg-tools
RUN tar -xzf /tmp/amneziawg-tools.tar.gz --strip-components=1 \
    && make -C src LDFLAGS=-static \
    && install -D -m 0755 src/wg /out/awg \
    && install -D -m 0755 src/wg-quick/linux.bash /out/awg-quick \
    && /out/awg --version

FROM docker.io/library/alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

ARG AWG_GO_VERSION
ARG AWG_GO_COMMIT
ARG AWG_TOOLS_VERSION
ARG AWG_TOOLS_COMMIT
ARG IMAGE_VERSION=dev
ARG IMAGE_REVISION=
ARG IMAGE_SOURCE=
ARG BUILD_DATE=

LABEL org.opencontainers.image.title="AmneziaWG server" \
      org.opencontainers.image.description="Userspace AmneziaWG server (amneziawg-go and amneziawg-tools built from pinned upstream sources)" \
      org.opencontainers.image.licenses="MIT AND GPL-2.0-only" \
      org.opencontainers.image.version="${IMAGE_VERSION}" \
      org.opencontainers.image.revision="${IMAGE_REVISION}" \
      org.opencontainers.image.source="${IMAGE_SOURCE}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.base.name="docker.io/library/alpine:3.24.4" \
      amneziawg-go.version="${AWG_GO_VERSION}" \
      amneziawg-go.revision="${AWG_GO_COMMIT}" \
      amneziawg-tools.version="${AWG_TOOLS_VERSION}" \
      amneziawg-tools.revision="${AWG_TOOLS_COMMIT}"

RUN apk add --no-cache bash dumb-init iproute2 iptables ip6tables nftables

COPY --from=go-builder /out/amneziawg-go /usr/local/bin/amneziawg-go
COPY --from=tools-builder /out/awg /out/awg-quick /usr/local/bin/
COPY rootfs/ /

RUN chmod 0755 /usr/local/bin/awg-* \
    && chmod 0644 /usr/local/share/amneziawg/lib.sh \
    && ln -s awg /usr/local/bin/wg \
    && ln -s awg-quick /usr/local/bin/wg-quick \
    && printf '%s\n' \
      "amneziawg-go=${AWG_GO_VERSION}" \
      "amneziawg-go-commit=${AWG_GO_COMMIT}" \
      "amneziawg-tools=${AWG_TOOLS_VERSION}" \
      "amneziawg-tools-commit=${AWG_TOOLS_COMMIT}" \
      >/usr/local/share/amneziawg/versions

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/local/bin/awg-entrypoint"]
HEALTHCHECK --interval=20s --timeout=5s --start-period=15s --retries=3 CMD ["/usr/local/bin/awg-healthcheck"]
