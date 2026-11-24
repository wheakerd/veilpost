# Soft Router Docker reference configurations

English | [简体中文](README.zh-CN.md)

This directory holds 36 VPN deployment configurations and supporting container files, for reference only. The files come from a private deployment setup, with private domains, private host paths, original project paths, and hosting provider identifiers removed; the remaining structure, images, ports, and semantics are unchanged. The size and SHA-256 of each file are recorded in `source-manifest.json`.

| Subdirectory                 | Contents                                                                                        |
| ---------------------------- | ----------------------------------------------------------------------------------------------- |
| `wire-guard/`                | AWG / WebSocket Compose for standard Docker, initialization override, Caddy, environment example, and nft entry |
| `wg-easy/`                   | Compose, Caddy, daemon, systemd, and nft entry for standalone wg-easy                           |
| `amneziawg-3.1/`             | Compose / SELinux overrides, environment example, Dockerfile, rootfs entry scripts, Docker systemd units |
| `awg31-reality-stack/`       | AWG / Xray Compose, daemon, configuration templates, sysctl, and systemd                        |
| `chain-reuse-v1/`            | Relay candidate and production override Compose                                                 |
| `reality-chain/`             | Historical REALITY egress Compose                                                               |
| `xray-reality-shadowrocket/` | Xray Compose and client / server templates                                                      |

## Placeholders

Before reuse, replace the placeholders with values for the target environment.

| Placeholder                                                                                                                                                       | Location                                               | Meaning                                                                                                                                                      |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `panel.example.com`                                                                                                                                               | `wg-easy/Caddyfile`                                    | Public domain of the wg-easy admin panel; Caddy uses it to request a TLS certificate                                                                         |
| `PANEL_DOMAIN`, `TUNNEL_DOMAIN`                                                                                                                                   | `wire-guard/.env.example`, `compose.yaml`, `Caddyfile` | Public domains of the admin panel and wstunnel, set in `.env`; Compose passes them as `${PANEL_DOMAIN:?…}` and Caddy reads them as `{$PANEL_DOMAIN}`; the example values `*.example.invalid` do not resolve |
| `${VAR:?…}`, `${VAR:-default}`                                                                                                                                    | Each Compose file                                      | Compose variable interpolation: `:?` refuses to start when the variable is missing, and `:-` uses the default when it is unset                               |
| `AWG_ENDPOINT_HOST=CHANGE_ME`                                                                                                                                     | `amneziawg-3.1/config/deployment.env.example`          | Public server address or domain that clients connect to                                                                                                      |
| `__SERVER_ADDRESS__`, `__SERVER_PORT__`, `__CLIENT_UUID__`, `__REALITY_PRIVATE_KEY__`, `__REALITY_PUBLIC_KEY__`, `__SHORT_ID__`, `__SOCKS_PORT__`, `__DNS_PORT__` | Xray configuration templates                           | Server address, port, client UUID, REALITY key pair, shortId, and local test ports written when the deployment process renders the templates; the repository contains no real values |

The following conventional values are not private information and are kept as is; adjust them as needed when reusing:

- Upstream images and digests (`ghcr.io/wg-easy/wg-easy`, `ghcr.io/xtls/xray-core`, `caddy`, `golang`, `alpine`, and others) and upstream source URLs.
- Container data and runtime directory conventions such as `/var/lib/<component>` and `/run/<component>`; `/opt/wg-easy/` and `/opt/soft-router/awg31-reality-stack/` are installation directory conventions for the standalone Docker runtime, referenced by the systemd units and `daemon.json`.
- Local image names, Compose labels, and nft rule comments with the `soft-router` prefix, used to identify resources created by these reference configurations.
- Private subnets (`10.42.42.0/24`, `10.79.31.0/24`, and others) and IPv6 ULA prefixes; for production deployments, generate your own ULA prefix according to RFC 4193.
- The REALITY camouflage target `dl.google.com` and the public DNS `1.1.1.1`.

## Boundaries

- Contains only configurations and the container scripts they use directly; no SSH credentials, real `.env` files, VPN private keys, runtime state, databases, logs, offline images, or binary tools.
- Third-party dependency caches and Docker examples from upstream sources are not part of this project's deployment configurations and are not included.
- This project's Compose development environment does not start or modify these configurations; they cannot be used to infer whether any VPN service is currently online. Do not merge and run these Compose files directly.

## Materials still needed before running

- `wire-guard`: `manage.py`, mentioned by the Compose file, is the original deployment tool and is not included. The private bootstrap environment file, wstunnel restrictions, and persistent directories must be prepared separately.
- `wg-easy`: replace the Caddy domain, and adjust the `/var/lib/wg-easy` and `/opt/wg-easy` directories for the target host.
- `amneziawg-3.1`: the two `artifacts/sources/*.tar.gz` source archives that the Dockerfile needs are not included, so the offline image build cannot be completed on its own; the rootfs scripts are kept. You can use `docker/node/amneziawg/` instead, which builds from pinned upstream commits and does not need these two archives.
- `awg31-reality-stack`: needs AWG / Xray image identifiers, a private `service.env`, the server runtime configuration, and a standalone Docker runtime.
- Xray / REALITY Compose files: the mounted `/var/lib/...` server directories and real keys are not in the repository, and the configuration templates must be filled in by the deployment process.
- The systemd files are text references and have not been written to the system service directory.

## Verification

From the project root, run `python3 scripts/verify-docker-copy.py` to confirm that every file in the manifest exists with a matching size and SHA-256, and that the directory contains no unlisted files (except this README, its Chinese translation, and the manifest itself).

After an intentional change to a reference file, run `python3 scripts/verify-docker-copy.py --update` to regenerate the manifest, and review the manifest diff before committing.
