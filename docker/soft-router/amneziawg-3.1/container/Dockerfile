# syntax=docker/dockerfile:1.7

FROM docker.io/library/golang:1.25.12-alpine3.23@sha256:f128118f1f3a7f38949c57f03d29d13e4afdb06b86f840f8f43e8031e6c1a73a AS awg_go_builder

RUN apk add --no-cache build-base git
COPY artifacts/sources/amneziawg-go-v3.1.20260814.tar.gz /tmp/amneziawg-go.tar.gz
RUN mkdir -p /src/amneziawg-go \
    && tar -xzf /tmp/amneziawg-go.tar.gz -C /src/amneziawg-go --strip-components=1 \
    && sed -i 's/const Version = .*/const Version = "v3.1.20260814"/' /src/amneziawg-go/version.go \
    && cd /src/amneziawg-go \
    && go mod download \
    && go mod verify \
    && if ! go test ./... >/tmp/amneziawg-go-test.log 2>&1; then tail -n 200 /tmp/amneziawg-go-test.log; exit 1; fi \
    && go build -trimpath -ldflags='-s -w -linkmode external -extldflags "-fno-PIC -static"' -o /out/amneziawg-go .

FROM docker.io/library/alpine:3.23.3@sha256:59855d3dceb3ae53991193bd03301e082b2a7faa56a514b03527ae0ec2ce3a95 AS awg_tools_builder

RUN apk add --no-cache build-base linux-headers
COPY artifacts/sources/amneziawg-tools-v3.1.20260812.tar.gz /tmp/amneziawg-tools.tar.gz
RUN mkdir -p /src/amneziawg-tools \
    && tar -xzf /tmp/amneziawg-tools.tar.gz -C /src/amneziawg-tools --strip-components=1 \
    && make -C /src/amneziawg-tools/src WITH_WGQUICK=yes

FROM docker.io/library/alpine:3.23.3@sha256:59855d3dceb3ae53991193bd03301e082b2a7faa56a514b03527ae0ec2ce3a95

ARG BUILD_DATE
ARG IMAGE_REVISION="local"

LABEL org.opencontainers.image.title="Portable AmneziaWG 3.1 gateway" \
      org.opencontainers.image.description="Userspace AmneziaWG 3.1 dual-stack full-tunnel gateway" \
      org.opencontainers.image.source="https://github.com/amnezia-vpn/amneziawg-go" \
      org.opencontainers.image.version="3.1.20260814-r4" \
      org.opencontainers.image.revision="${IMAGE_REVISION}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      io.soft-router.awg-go.commit="1b86b2ae0e493e7ea93f8c1a0f0cb6735b1551f1" \
      io.soft-router.awg-tools.commit="ee0f0a9aa34ff0a0da4b3433b9512781cfe02843"

RUN apk add --no-cache bash dumb-init iproute2 iptables iputils nftables

COPY --from=awg_go_builder /out/amneziawg-go /usr/local/bin/amneziawg-go
COPY --from=awg_tools_builder /src/amneziawg-tools/src/wg /usr/local/bin/awg
COPY --from=awg_tools_builder /src/amneziawg-tools/src/wg-quick/linux.bash /usr/local/bin/awg-quick
COPY container/rootfs/ /

RUN chmod 0755 \
      /usr/local/bin/amneziawg-go \
      /usr/local/bin/awg \
      /usr/local/bin/awg-quick \
      /usr/local/bin/awg-add-client \
      /usr/local/bin/awg-entrypoint \
      /usr/local/bin/awg-firewall \
      /usr/local/bin/awg-healthcheck \
      /usr/local/bin/awg-init-state \
      /usr/local/bin/awg-restore-sysctls \
    && ln -s /usr/local/bin/awg /usr/local/bin/wg \
    && ln -s /usr/local/bin/awg-quick /usr/local/bin/wg-quick

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/local/bin/awg-entrypoint"]
HEALTHCHECK --interval=20s --timeout=5s --start-period=15s --retries=3 CMD ["/usr/local/bin/awg-healthcheck"]
