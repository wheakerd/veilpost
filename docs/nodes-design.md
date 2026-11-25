# Node feature design (draft)

English | [简体中文](nodes-design.zh-CN.md)

The implemented control channel covers node enrollment, online status, and Docker Engine installation on opted-in hosts. Docker jobs select a locally registered package source and carry a typed result through the signed poll. [API contract](../app/frontend/docs/nodes-api.md)

## Goals and scope

- Resistance to active probing: unauthenticated probes see only a real website or get no response at all.
- Resistance to DPI and traffic disguise: handshake appearance and packet-length patterns resemble common legitimate traffic.
- No single-protocol long-lived connections: each node offers TCP, HTTP, and UDP entry points at the same time; underlying connections are replaced periodically, and clients can switch between entry points.
- Scale: personal use, a small number of nodes (10 or fewer), one or two administrators. No billing, quotas, or multi-tenancy.
- Open source: code and documentation contain no private domains, addresses, keys, or deployment paths; all sites, ports, and parameters are configured by the user or generated randomly on the node.

Non-goal: guaranteeing that the traffic is not identified. Rotation and disguise raise the cost of identification, but long-term heavy traffic to a single IP is itself a signature; neither the documentation nor the interface promises "undetectable".

## Overall structure

```text
Browser ──/api/auth, /api/nodes──▶ Console (app/backend)
                                      ▲
                                      │ HTTPS, initiated by the node, signed requests
                                      │
Node ── veilpost-agent (app/agent) ──manages──▶ Xray / Hysteria2 / AmneziaWG containers
```

- `app/agent/`: a new Go program, `veilpost-agent`; CGO-free, a single binary, distributed with the release packages.
- Nodes do not open a management port. The agent pulls the desired configuration from the console and reports status through long polling: the server holds a request for up to 25 seconds, and the agent reconnects immediately after it returns.
- The console adds `/api/agent/*` (called by nodes, signature authentication) and `/api/nodes/*` (called by the browser, using the existing session authentication and CSRF rules).
- Services on a node run as Docker containers, consistent with the `docker/soft-router/` reference configuration; the agent generates Compose files and calls `docker compose`, and holds no privileges beyond those of the Docker daemon.

## Control channel

### Enrollment

1. An administrator creates a node in the console and receives a one-time enrollment token (random 256 bits, valid for 15 minutes, only its hash is stored in the database).
2. On the node, run `veilpost-agent enroll --server https://console.example --token-stdin`; the token is passed on standard input.
3. The agent generates an Ed25519 key pair locally and submits the public key, together with a signature over the token hash and the public key made with the new private key as proof of possession; the public key is unique within the console. The console consumes the token and binds the node. The private key is stored only in a `0600` file on the node.

### Request authentication

- Every request carries an Ed25519 signature over `vpn-agent-v1`, the HTTP method, the API path, the node ID, a timestamp, a nonce, and the SHA-256 of the request body; requests with a clock skew greater than 60 seconds are rejected, and nonces are deduplicated within the validity window to prevent replay.
- Signatures rather than mTLS: the console can sit behind Nginx or a CDN without running its own CA.
- The console can revoke a node; after revocation its signatures stop working immediately, and reconnecting requires a new enrollment token.
- `/api/agent/*` has its own per-node rate limit that does not share quota with the sign-in rate limit.

### Configuration delivery

- The console keeps an incrementing configuration version for each node; the agent long-polls with its current version and fetches the complete desired state when a newer version exists.
- The agent renders the configuration locally and first checks it with each service's own validation (such as `xray run -test`); only when that passes does it replace the configuration and restart the corresponding container. On failure it keeps the old configuration and reports the error.

## Key ownership

| Data                                            | Generated on | Stored by the console                                                  |
| ----------------------------------------------- | ------------ | ---------------------------------------------------------------------- |
| REALITY private key                             | Node         | Public key only                                                        |
| AmneziaWG server private key                    | Node         | Public key only                                                        |
| AmneziaWG client private key                    | Console      | Encrypted with `master.key`, decrypted when exporting a client profile |
| VLESS UUID, REALITY shortId, Hysteria2 password | Console      | Encrypted with `master.key`; also delivered to the node                |
| Node signing private key                        | Node         | Public key only                                                        |

Client credentials must be exportable again, so they can only be stored encrypted, not as hashes alone.

## Entry protocols

Each node enables the following four entry points by default, with configurable ports; the parameters of each protocol are generated randomly per node, so that deployments do not all share the same set of characteristic values.

### 1. VLESS + REALITY + Vision (TCP)

- Default TCP 443; `flow: xtls-rprx-vision`; client uTLS fingerprint `chrome`.
- The borrowed site (`target` and `serverNames`) is entered by the administrator; no default is provided. The agent checks the candidate site from the node: TLS 1.3, HTTP/2, X25519, no redirects; if any is not met, it refuses to enable the entry point.
- Each client has its own shortId and can be revoked individually.

### 2. VLESS + XHTTP + REALITY (TCP)

- A separate port (a random high port by default, which can be changed to 8443 or similar); it also uses REALITY, and its borrowed site can differ from entry point 1.
- The core of avoiding long-lived connections: client XMUX parameters are randomized within ranges, limiting how many times and for how long a single underlying connection is reused; when the limit is reached a new connection is established automatically. For example:
    - `cMaxReuseTimes`, `hMaxRequestTimes`, and `hMaxReusableSecs` use range values;
    - use either `maxConcurrency` or `maxConnections`, not both.
- `mode` is set explicitly instead of `auto` (there are existing reports of `auto` failing to connect with REALITY).
- Optional in a later phase: use `downloadSettings` to send uplink and downlink through different addresses or entry points.

### 3. Hysteria2 (UDP)

- Default UDP 443; it shares the port number with entry point 1 without conflict.
- The server uses the official `tobyxdd/hysteria` image (published from the `apernet/hysteria` repository). Xray's built-in Hysteria2 inbound is a new feature with existing reports of handshake failures; it is not used for now and will be reevaluated later.
- Two disguise modes, chosen per node:
    - Masquerade (default): looks like HTTP/3, unauthenticated requests are answered by the masquerade site, resisting active probing;
    - Salamander obfuscation: looks like random UDP and exposes no QUIC characteristics, but cannot answer probes.

### 4. AmneziaWG (UDP)

- The server image lives in `docker/node/amneziawg/`: it builds `amneziawg-go v3.1.20261116` and `amneziawg-tools v3.1.20260812` from pinned upstream commits, verifies the SHA-256 of the source archives, and pins the base image by digest. Currently the container generates the server key and the first client locally; integrating with `veilpost-agent` requires an additional startup mode in which configuration and peers are provided externally, to comply with the "Key ownership" table, and the shared obfuscation parameters (including HeaderProtectionKey, stored encrypted with `master.key`) must be reported to the console.
- Uses the 2.0 parameters: Jc/Jmin/Jmax, S1–S4, and H1–H4 ranges (non-overlapping) are generated randomly per node; the I1–I5 pre-handshake signature packets are chosen from templates (such as QUIC or DNS appearance). The image already generates S1–S4 and non-overlapping H1–H4 ranges randomly per node and validates I1–I5 variables and tags; I packet templates are not provided yet.
- The port is left empty by default, and a random high UDP port is chosen per node at deployment; UDP 443 is reserved for Hysteria2.
- It does not respond to unauthenticated packets, so the port is invisible to port scans.

## Client configuration export

Each client (device) gets one configuration containing all enabled entry points of the node and a switching policy:

| Client                     | Format                     | Entry points                                                              | Switching policy                        |
| -------------------------- | -------------------------- | ------------------------------------------------------------------------- | --------------------------------------- |
| Xray / v2rayN family       | Xray JSON, share links     | 1, 2, 3                                                                   | `balancer` + `observatory`              |
| Clash Meta (mihomo) family | YAML                       | 1, 2, 3, 4                                                                | `url-test` / `fallback` groups          |
| sing-box family            | JSON                       | 1, 3 (the official build does not support XHTTP; AmneziaWG only in forks) | `urltest` group                         |
| Shadowrocket               | Share links / subscription | 1, 3; XHTTP support to be verified                                        | Determined by the client's group policy |
| AmneziaVPN                 | `.conf`                    | 4                                                                         | —                                       |

Field support in each client follows the versions current at implementation time; each format's generator is tested separately.

Export methods:

- By default, files are downloaded or a QR code is shown after signing in to the console; credentials never appear in URLs, logs, or browser storage.
- Subscription links (optional, off by default): each client has its own random token, stored only as a hash in the database and revocable individually; the link itself is a credential, which the interface must state clearly.

## Monitoring

The agent attaches status to every poll, and the nodes page shows only this real data:

- Node: online status (the last verified signed request was within 90 seconds), agent version, configuration version, apply errors.
- Entry points: container status, port listening, reachability of the REALITY borrowed site.
- Traffic: the Xray stats API, the Hysteria2 traffic API, and per-client bytes sent and received and latest handshake time from `awg show`.

When something is not connected or data is missing, "Unknown" is shown; no default values are filled in.

## Data model (SQLite)

- `nodes`: name, public address, status, signing public key, configuration version, last report time.
- `node_enrollments`: token hash, node, expiry time, whether used.
- `inbounds`: node, type, port, parameters JSON (without node private keys).
- `clients`: name, note, enabled state.
- `client_credentials`: client × entry point, encrypted credentials.
- `node_reports`: the latest status report; only the newest one and a limited history are kept.

## Interface

- Node list: real status, health of each entry point, traffic.
- Node details: entry point toggles and parameters, the enrollment command (the token is shown only once), reissuing a token for a node that has not enrolled, revocation, deleting revoked nodes.
- Clients: create, enable/disable, export by format, QR codes.
- All write operations use the existing session authentication; sensitive operations (creating a node and issuing an enrollment token, revoking a node, exporting configuration) require entering the password again.

## Testing

- Go: golden-file tests for configuration rendering; unit tests for signatures, replay, revocation, and token consumption; where available, CI uses the official images to validate the generated configuration with `xray run -test` and similar checks.
- End-to-end: the console and a real `veilpost-agent` tested together. The agent needs a mode of operation that does not start containers and only renders and validates the configuration; this is a test-only code path, and its exact form needs to be confirmed (see below).
- Do not mock the console API, and do not add test-only endpoints to production code.

## Phased implementation

1. Node enrollment and the control channel: enrollment tokens, signed requests, long polling, online status; the nodes page shows real status.
2. VLESS + REALITY + Vision and XHTTP entry points: configuration delivery, validation, health checks.
3. Hysteria2 and AmneziaWG entry points.
4. Client management and export in five formats.
5. Traffic statistics and the monitoring page.

Each phase is usable on its own and comes with tests and documentation updates.

## Decisions (2026-11-22)

1. End-to-end tests between the agent and the console are not implemented for now; Go unit tests and golden-file tests come first, and the testing approach will be decided later.
2. Keep the subscription link feature, off by default.
3. Build the AmneziaWG image ourselves: build from source at pinned upstream versions and verify hashes; implemented in `docker/node/amneziawg/` on 2026-11-22.
4. Before open-sourcing, remove private deployment information (domains, absolute paths, source paths) from `docker/soft-router/`; the working tree was cleaned on 2026-11-22, and on 2026-11-23 the history was replaced by one parentless commit, force-pushed once to `main` after the maintainer confirmed the repository and branch.
5. Revoked nodes can be deleted; for a node that has not enrolled, a token can be reissued after it expires or is lost, and the old token becomes invalid at once (implemented 2026-11-22).

## Open questions

1. Shadowrocket support for XHTTP and AmneziaWG; to be confirmed by testing before implementing phase 4.
