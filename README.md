# Veilpost

A self-hosted control console for multi-protocol VPN nodes.

English | [简体中文](README.zh-CN.md)

A Vue frontend, a Go + Gin backend, and a Docker Compose development environment. Sign-in, session restoration, TOTP two-step verification, recovery codes, and account security settings all call the real backend; there is no default account and no mock authentication. The interface uses Liquid Glass after iOS 26, with light and dark modes and eight theme colors. The sign-in page shows a slowly rotating wireframe globe, and the console has collapsible groups, a sidebar that can be resized by dragging, a toolbar, and a tab bar of open pages. The "Preferences" drawer offers theme color, vision mode (including color-safe and grayscale), liquid glass, page transitions, the tab bar, the loading bar, page state caching, and other options. The interface supports 简体中文, English, 繁體中文, and Русский: it follows the browser language by default, can be switched in "Preferences", and the choice is saved only in the current browser. The software store enables VPN software from the built-in catalog shipped with the console (Xray-core, Hysteria 2, AmneziaWG) and configures it in the console; deployment to nodes will come with node phase 2. Certificates are requested through ACME (Let's Encrypt by default, with a custom directory and EAB available) and DNS-01 and renewed automatically, with support for Cloudflare, Alibaba Cloud DNS, Tencent Cloud DNSPod, and AWS Route 53. Credentials and private keys are stored encrypted with `master.key`, and the backend needs access to the ACME service and the API of the chosen DNS provider. Node phase 1 is implemented: create a node on the "VPN nodes" page to get a one-time enrollment token, and once `veilpost-agent` on the node enrolls, it reports its online status through signed long polling.

## Continue development with Docker

```bash
make start          # Build and start the frontend and backend, wait for health checks, open the default browser
make status         # Show container status and the actual access URL
make admin-create   # Interactive creation in a terminal; a random password is generated in non-interactive environments
make admin-reset-password USERNAME=admin   # Reset a forgotten password and revoke all sessions of the account
make admin-reset-two-factor USERNAME=admin # Remove two-step verification when both the authenticator and recovery codes are lost
make logs
make restart        # Rebuild and recreate the containers, keeping account and session data
make stop           # Stop this project's containers, keeping the data volume
```

The default entry point is <http://127.0.0.1:3000>. Use `FRONTEND_PORT=0 make start` to let Docker assign the frontend host port; `OPEN_BROWSER=false make start` turns off opening the browser automatically. Frontend source is bind-mounted to provide HMR; after changing backend source, run `make restart`.

In a terminal you can also run `make admin-create GENERATE_PASSWORD=1 USERNAME=admin` to generate a 64-character random password and create the account. This command runs POSIX shell directly and does not depend on Python; you can also run `GENERATE_PASSWORD=1 sh scripts/admin-create.sh`. The password is generated with the system `openssl rand -hex 32`. When a tool is missing, the script reads the Linux `os-release`, installs the required OpenSSL/coreutils through apt-get, dnf/yum, apk, pacman, or zypper, then checks the tools again and continues. Installation needs root or a working sudo; non-interactive runs use `sudo -n`, and the script exits with a clear message when permissions are insufficient or installation fails.

In generated mode the account name defaults to `admin`. No account is preset, and existing accounts are never overwritten and their passwords never reset. The password is passed to the backend on standard input and saved only to the text file under `.runtime/admin-credentials/` that the command output points to (directory mode `700`, file mode `600`, ignored by Git); it is not printed to the terminal or logs. `VPN_CREDENTIALS_DIR` can name another private directory. Save the password in a password manager, then delete the local plaintext file. If the creation result is not confirmed, the file is kept and the command does not retry automatically. Password resets use the same password input and storage method.

When running the binary directly in production, set `VPN_API_BIN` and a `VPN_DATA_DIR` that matches the service; release packages include the same shell script, so Python, Node, and the Go SDK do not need to be installed. See the [release documentation](docs/releasing.md) for examples. The other development and packaging commands still use Python.

Containers communicate through `backend:8080`; the backend's host port is assigned by Docker and bound to `127.0.0.1` only. The project name defaults to `veilpost-dev`, and the stack does not start or modify the reference deployments in `docker/soft-router/`.

The Make launcher uses the project's existing Go module cache to speed up image builds and verifies it again inside the image; when Docker Compose is used directly, locked dependencies are fetched from the network starting with an empty cache.

Authentication data, including the SQLite database and the encryption key, is stored in a separate `auth-data` named volume. Recreating containers does not clear user data. Do not delete this volume; for backups, stop the backend and save both the database and `master.key`.

## Checks, builds, and releases

```bash
make install
PLAYWRIGHT_CHANNEL=chrome make check
make build
make release VERSION=v0.1.0
```

Local checks need Go, Node, Bun, and Python; see each subproject's configuration for versions. Browser tests start a real Go + SQLite service with a separate temporary database and random test accounts; there is no MSW or Playwright request interception.

GitHub Actions runs checks on branch pushes and PRs. After a version tag is pushed, the frontend package, the Linux amd64 / arm64 backend packages, and checksums are published only if the checks pass. See [Development and releases](docs/releasing.md) for details.

## Native development

When you need native debugging, use `make native-start`, `make native-status`, `make native-restart`, and `make native-stop`. The backend uses a random port assigned by the system, which is then passed to the frontend on the command line; native mode uses a data directory separate from Compose. Both modes use frontend port 3000 by default, so stop one mode before switching to the other.

## Structure and boundaries

```text
app/frontend/     Vue interface, real API clients, browser tests
app/backend/      Gin API, SQLite authentication and node storage, administrator commands and tests
app/agent/        veilpost-agent node agent (Go, no CGO)
compose.yaml      Frontend and backend development orchestration, separate network and named volume
docker/frontend/  Frontend development image and Nginx reference
docker/backend/   Go build and backend runtime image
docker/soft-router/  Sanitized VPN deployment reference configuration
docker/node/      Node images (AmneziaWG built from pinned upstream sources; not started by the development environment)
scripts/          Start/stop, test, and release scripts
.github/          CI and tag release workflows
```

The real backend currently covers authentication, health checks, and node enrollment and online status; VPN configuration delivery, entry protocols, and monitoring are not implemented yet. When there are no nodes, the nodes page keeps its real not-connected state and shows no fictional data. See the [node feature design](docs/nodes-design.md).

Reference documents: [frontend](app/frontend/README.md), [backend](app/backend/README.md), [authentication contract](app/frontend/docs/auth-api.md), [node contract](app/frontend/docs/nodes-api.md), [agent](app/agent/README.md). The placeholders in the VPN deployment reference configuration are described in [docker/soft-router](docker/soft-router/README.md), and file integrity can be checked with `python3 scripts/verify-docker-copy.py`.
