# AmneziaWG node image

English | [简体中文](README.zh-CN.md)

Builds an AmneziaWG server image from upstream sources pinned by commit: the user-space daemon `amneziawg-go` (Go) and the management tools `awg` and `awg-quick` (C, from `amneziawg-tools`). On first start, the container randomly generates the server keys, the first client configuration, and the obfuscation parameters locally on the node; neither the image nor the repository contains any keys or client configurations.

This directory is only the node-side image. It is not part of the development Compose environment, and `make start` neither builds nor starts it. `docker/soft-router/amneziawg-3.1/` is its reference source (it lacks the source archives and cannot be built on its own); the container scripts were copied from there and modified.

## Upstream versions

| Component         | Version         | Commit                                     | Archive SHA-256                                                    |
| ----------------- | --------------- | ------------------------------------------ | ------------------------------------------------------------------ |
| `amneziawg-go`    | `v3.1.20261116` | `08fe3d33bbb116be09a6183d02609311b8b0c9e8` | `e33d8a62928222592df23b75356d79e0ea6b590943cbe3a4cfcdf9c357bb416e` |
| `amneziawg-tools` | `v3.1.20260812` | `ee0f0a9aa34ff0a0da4b3433b9512781cfe02843` | `22438f231d39ea27e4bdc69707ec3103357f3cecc9ad3eecba18a61e5b39c9b9` |

- Sources are fetched from `https://github.com/amnezia-vpn/<repository>/archive/<commit>.tar.gz` and verified by `ADD --checksum` before use; the build fails if the hash does not match.
- The base images (`golang:1.27.4-alpine3.24`, `alpine:3.24.4`) and the Dockerfile frontend (`docker/dockerfile:1.29.0`) are all pinned by digest.
- The build runs all upstream Go tests; the tests need cgo (a dependency of the upstream `outline` package), while the daemon itself is compiled statically with `CGO_ENABLED=0`. `awg` is linked with `-static`.
- Version information is written to image labels (`amneziawg-go.version`, `amneziawg-go.revision`, `amneziawg-tools.version`, `amneziawg-tools.revision`) and to `/usr/local/share/amneziawg/versions` inside the container, which is copied to `meta/versions` in the state directory during initialization.

When upgrading upstream, the version, commit, and SHA-256 at the top of the Dockerfile must be changed together. To compute a new hash:

```sh
curl -fsSL https://github.com/amnezia-vpn/amneziawg-go/archive/<commit>.tar.gz | sha256sum
```

Before upgrading, confirm that the UAPI keys of `amneziawg-tools` and `amneziawg-go` match, and rerun the checks below.

## Build

Run in this directory:

```sh
docker build -t veilpost-amneziawg:dev .
```

Requires BuildKit (enabled by default in Docker 23 and later) and network access to GitHub, the Go module proxy, and Docker Hub. Optional build arguments: `IMAGE_VERSION`, `IMAGE_REVISION`, `IMAGE_SOURCE`, and `BUILD_DATE`, written to the corresponding `org.opencontainers.image.*` labels.

Smoke checks that need no privileges:

```sh
docker run --rm --network none --entrypoint awg veilpost-amneziawg:dev --version
docker run --rm --network none --entrypoint amneziawg-go veilpost-amneziawg:dev --version
```

## Runtime requirements

- Kernel: any Linux kernel with TUN support; the AmneziaWG kernel module is not needed (if the host has the `amneziawg` module loaded, `awg-quick` prefers it). `/dev/net/tun` is required.
- Network: `network_mode: host`. The container creates the interface, opens the UDP listening port, and installs forwarding and NAT rules for the tunnel subnet in the host network namespace.
- Container capabilities: `NET_ADMIN` (creating interfaces, routes, iptables/nftables), `NET_RAW` (iptables), and `NET_BIND_SERVICE` (listening on ports below 1024, such as 443). All other capabilities are dropped; `--privileged` is not needed.
- Host sysctls: the container does not modify host sysctls; the host sets them before start:
  - `net.ipv4.ip_forward=1`;
  - when IPv6 is enabled (`AWG_REQUIRE_IPV6=true`), also `net.ipv6.conf.all.forwarding=1`; if the IPv6 default route comes from router advertisements, the egress interface needs `accept_ra=2`.
- Firewall: the scripts insert the `AWG_IN`, `AWG_FWD`, and `AWG_NAT` chains (`AWG_IN6`, `AWG_FWD6`, and `AWG_NAT6` for IPv6) and remove them when the container stops. The iptables in the image uses the nf_tables backend; if the host still uses iptables-legacy, the rules end up in a different set of tables, which you need to verify yourself. When the host has native nftables `input`/`forward` chains (for example, a custom `inet filter` table), `AWG_NATIVE_NFT_MODE=auto` inserts accept rules commented `amneziawg-node-*` at the head of those chains and removes them by comment on stop, so that the drop policies of those chains do not block tunnel traffic.
- The root filesystem is read-only; `/run` and `/tmp` use tmpfs. The container runs as root (required to create the interface).

## State directory

`/var/lib/amneziawg` inside the container, bound to `AWG_STATE_DIR` on the host in the example. Create this directory in advance (for example, `install -d -m 0700 -o root -g root <directory>`) and include it in backups. Contents:

| Path                       | Contents                                                                                           |
| -------------------------- | -------------------------------------------------------------------------------------------------- |
| `server/<interface>.conf`  | Server configuration, including the server private key, preshared keys, and all `[Peer]` sections |
| `clients/<name>.conf`      | Client configuration, including the client private key; can be imported directly into clients such as AmneziaVPN |
| `meta/`                    | Server public key, interface name, first client name, public address, versions, initialization time |
| `rollbacks/`               | Server configuration saved by `awg-add-client` before each change                                  |

- All files have mode `0600` and directories `0700`. These files are all credentials; do not commit them to the repository or send them to logs.
- State is generated only on first start; every later start only validates it. If the interface name, first client name, or public address differs from the existing state, or the directory is incomplete, the container refuses to start and does not regenerate keys. To change the address, first back up and empty the state directory, then redistribute the client configurations.
- The image contains no script to restore host sysctls (`awg-restore-sysctls` in the reference configuration depends on the original deployment tool and was not copied).

## Environment variables

| Variable                            | Default                         | Description                                                                                   |
| ----------------------------------- | ------------------------------- | --------------------------------------------------------------------------------------------- |
| `AWG_ENDPOINT_HOST`                 | None (required)                 | Public domain name or IP that clients connect to, written to `Endpoint` in client configurations |
| `AWG_LISTEN_PORT`                   | `443`                           | UDP listening port                                                                            |
| `AWG_INTERFACE`                     | `awg0`                          | Interface name (at most 15 characters)                                                        |
| `AWG_MTU`                           | `1280`                          | Tunnel MTU, 1280–1420                                                                         |
| `AWG_SERVER_IPV4`, `AWG_SERVER_IPV6` | `10.77.0.1/24`, `fd77:77:77::1/64` | Server tunnel addresses                                                                    |
| `AWG_CLIENT_IPV4`, `AWG_CLIENT_IPV6` | `10.77.0.2/32`, `fd77:77:77::2/128` | First client addresses                                                                    |
| `AWG_IPV4_SUBNET`, `AWG_IPV6_SUBNET` | `10.77.0.0/24`, `fd77:77:77::/64` | Tunnel subnets matched by the forwarding and NAT rules; should agree with the server addresses |
| `AWG_CLIENT_NAME`                   | `client1`                       | File name of the first client configuration                                                   |
| `AWG_CLIENT_DNS`                    | `1.1.1.1, 2606:4700:4700::1111` | DNS written to client configurations; accepts IPs only                                        |
| `AWG_CLIENT_I1` … `AWG_CLIENT_I5`   | Empty                           | Client pre-handshake signature packets; see below                                             |
| `AWG_REQUIRE_IPV6`                  | `false`                         | When `true`, requires the host to have an IPv6 default route and a global address, and installs IPv6 forwarding and NAT rules |
| `AWG_NATIVE_NFT_MODE`               | `auto`                          | `auto`: insert accept rules when the host has the matching nftables chains; `required`: the chains must exist; `off`: do nothing |
| `AWG_NATIVE_NFT_FAMILY`, `_TABLE`, `_INPUT_CHAIN`, `_FORWARD_CHAIN` | `inet`, `filter`, `input`, `forward` | Location of the host's native nftables chains                                |
| `LOG_LEVEL`                         | Unset                           | `amneziawg-go` log level: `error`, `verbose` (or `debug`), `silent`; the example Compose sets `error` |

The ULA prefix `fd77:77:77::/64` is only an example; for production deployments, randomly generate your own prefix according to RFC 4193.

## Obfuscation parameters

All values are generated randomly per node from `/dev/urandom` on first initialization, so different nodes do not share the same set of signature values. The upstream `amneziawg-go` README is authoritative for the meaning of the parameters.

| Parameter                            | Generation                                                                                                                       | Location               |
| ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------- | ---------------------- |
| `S1`, `S2`, `S3`, `S4`               | Taken from 48–96, 96–144, 48–96, and 12–32 respectively; `S1 + 56 ≠ S2` is guaranteed so that handshake initiation and response packets differ in length; header protection requires each to be at least 12 | Same on server and client |
| `H1`–`H4`                            | 5–4294967295 is split into four segments assigned in random order; each takes a random range 2^16–2^24 wide within its segment; the four ranges do not overlap each other or the standard WireGuard type values 1–4 | Same on server and client |
| `HeaderProtectionKey`                | `awg genkey`                                                                                                                     | Same on server and client |
| `RandomTrailers`, `DisableCookies`   | Fixed `on` and `off`; checked by the health check                                                                                | Same on server and client |
| `Jc`, `Jmin`, `Jmax`                 | Random per client: 4–8, 32–64, 256–512                                                                                           | Client only            |
| `I1`–`I5`                            | Taken from `AWG_CLIENT_I1`…`I5` and written as is into newly generated client configurations; not sent when empty                | Client only            |

`I1`–`I5` accept only the tags listed in the upstream documentation: `<b 0xhex>` (an even number of digits), `<r N>`, `<rd N>`, `<rc N>`, and `<t>`, with no other characters between tags and a total length of at most 4096. For example, to imitate the first packet of some protocol, you can write `<b 0x…fixed header…><r 32><t>`; specific templates (QUIC, DNS, and similar appearances) are not shipped with the image and are generated by the user or the console according to the upstream documentation. Keep the generated packet length within the path MTU, otherwise packets are fragmented.

These variables affect only client configurations generated afterward; existing client configurations are not rewritten.

## Adding clients

While the container is running and healthy, run `awg-add-client` inside it:

```sh
docker exec \
  -e AWG_NEW_CLIENT_NAME=phone \
  -e AWG_NEW_CLIENT_IPV4=10.77.0.3/32 \
  -e AWG_NEW_CLIENT_IPV6=fd77:77:77::3/128 \
  amneziawg awg-add-client
```

- `AWG_NEW_CLIENT_NAME` is at most 15 characters; the addresses must be unused `/32` and `/128` addresses. `AWG_NEW_CLIENT_DNS` defaults to `1.1.1.1`; set it to an empty string to omit DNS. `AWG_CLIENT_I1`…`I5` also apply.
- The script first replays the current configuration with `awg syncconf` to validate the rollback path, then applies the new `[Peer]`; on failure it restores the original configuration. The client configuration is written to `clients/<name>.conf`.

To export client configurations, copy the files directly from the state directory; do not pass their contents through `docker logs` or command-line arguments.

## Example Compose

`compose.example.yaml` is an example: copy it to the node, modify it as needed, and set at least `AWG_STATE_DIR` and `AWG_ENDPOINT_HOST` in a `.env` file in the same directory:

```sh
docker compose -f compose.example.yaml up -d --build
docker compose -f compose.example.yaml logs -f
```

SELinux hosts may need `label=disable` added to `security_opt`, or a suitable label set on the state directory.

## Boundaries

- This image currently runs standalone: the server keys and the first client are generated inside the container. The onboarding approach in `docs/nodes-design.md`, where the console generates client private keys and `veilpost-agent` delivers the configuration, is not yet implemented; at that point the scripts will need a startup mode in which the server configuration is provided externally.
- The checks for this directory cover only the image build, binary versions, and offline state generation; no tunnel was started and no client connected on a real node, so they cannot be used to infer that any node works.
