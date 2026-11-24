# veilpost-agent

English | [简体中文](README.zh-CN.md)

The node agent, corresponding to phase 1 of the [node design](../../docs/nodes-design.md): it enrolls with a one-time token, long-polls the console with Ed25519-signed requests, and reports its online status and version. In this phase it manages no containers and applies no VPN configuration. See [nodes-api.md](../frontend/docs/nodes-api.md) for the API contract.

It is implemented with the Go standard library only, has no third-party dependencies, and builds into a single binary with `CGO_ENABLED=0`. The Go version matches `app/backend/.go-version`, and the module path is `github.com/wheakerd/veilpost/app/agent`.

## Usage

1. Create a node on the "VPN nodes" page of the console; after you enter the current password, you get the enrollment command and a one-time token (valid for 15 minutes, shown only once).
2. Run the enrollment command on the node, paste the token when prompted, and press Enter:

    ```bash
    veilpost-agent enroll --server https://console.example --token-stdin
    ```

    The token is read only from standard input; command-line arguments and environment variables are not accepted. The server address must use HTTPS; only loopback addresses (local development) may use HTTP.

3. Stay connected:

    ```bash
    veilpost-agent run
    ```

`--state-dir` replaces the default state directory `/var/lib/veilpost-agent`. The directory mode is `0700`, and `identity.key` (Ed25519 private key, PKCS#8 PEM) and `node.json` (server address, node ID, name) inside it both have mode `0600`. When an identity already exists, `enroll` refuses to overwrite it; when enrollment fails, the newly generated private key is deleted. `run` refuses a directory or private key file that the group or other users can access. The private key never leaves the node.

Exit codes: `0` normal stop after SIGINT / SIGTERM; `1` runtime failure; `2` invalid arguments; `3` the node has been revoked in the console. When running as a system service, exit code 3 should not trigger an automatic restart (for example, systemd's `RestartPreventExitStatus=3`). To reconnect, create a new node in the console and enroll again.

While running, the agent starts the next long poll (up to 25 seconds each) as soon as the previous one ends; connection failures back off exponentially from 5 seconds to 5 minutes. Requests are rejected when the system clock differs from the console by more than 60 seconds, so enable NTP time synchronization. Logs go to standard error and contain no tokens or private keys.

## Development

```bash
make fmt     # gofmt
make vet
make test    # Unit tests with race detection
make build   # bin/veilpost-agent (CGO_ENABLED=0)
make check   # fmt-check, vet, test, build
```

The Makefile prefers the Go in the repository's `.runtime/toolchains/` and also accepts `GO=/path/to/go`; build and module caches live in the repository's `.runtime/`. The version is injected through `-ldflags "-X main.version=... -X main.commit=..."` and defaults to `dev`.

Tests live in `tests/agent/` and, as an external package, call only the public API; the protocol constants and signing functions they need are exported in `internal/agent/export.go`. The tests cover the signature format (pinned to the same known-answer signature vector as the backend), state directory and key file permissions, cleanup after failed enrollment, signed requests, backoff, and exit on revocation. End-to-end integration tests against a real console are not carried out in this phase.
