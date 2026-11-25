# Localization, software store, and certificate management design

English | [简体中文](platform-design.zh-CN.md)

Status: design confirmed (2026-11-22). Localization, the software store, and certificate management are implemented (2026-11-23). Localization: nothing is written to storage while no language has been chosen, and notification messages already on screen are not re-rendered when the language changes. Software store: settings are written with `POST /api/apps/:id/settings` (consistent with existing write requests), and deploying to nodes depends on node phase 2; the official Hysteria 2 image is `tobyxdd/hysteria`; the AmneziaWG port is random per node by default, and UDP 443 is reserved for Hysteria 2. Certificates: lego v4.35.2; a failed renewal backs off for 1, 2, 4, 8, and 16 hours and then retries daily, and a failed first issuance is not retried automatically; the status stays `valid` during renewal. The API contracts are in `app/frontend/docs/apps-api.md` and `certs-api.md`. The node feature design is in [nodes-design.md](nodes-design.md).

## Decisions

1. "Installing" a VPN software in the software store means enabling it in the console: its settings and management pages appear in the sidebar. Which nodes it is deployed to is chosen separately on the management page, and the node agent actually pulls the image and starts it (depends on node phase 2). Software that is deployed to nodes cannot be uninstalled directly; it must first be removed from all nodes.
2. The store uses only the built-in catalog released with the console version; images are pinned by digest. New VPN software is added with console upgrades; remote software sources and uploading custom packages are not supported.
3. Interface languages: 简体中文 (`zh-CN`), English (`en`), 繁體中文 (`zh-TW`), Русский (`ru`).
4. Certificates use the ACME protocol, Let's Encrypt by default, and a custom ACME directory URL is allowed (such as ZeroSSL or Google Trust Services, with EAB when required). Domain validation uses DNS-01, and the interface initially offers four providers: Cloudflare, Alibaba Cloud DNS, Tencent Cloud DNSPod, and AWS Route 53.

## 1. Localization

- Use `vue-i18n` (Composition API mode), with language packs split by feature: `src/locales/<language>/<feature>.ts`, so that several people do not edit the same file at once.
- Default language: on the first visit it matches the browser language, falling back to Simplified Chinese when nothing matches; users switch it in "Preferences", and the choice is saved in `src/stores/preferences.ts` (a non-sensitive preference that may be persisted).
- `<html lang>` follows the language; dates and numbers are formatted for the current language with `Intl`.
- The backend returns only error codes, not text; every error code has text in every language pack, and missing text falls back to Simplified Chinese with a warning in development builds.
- Element Plus components switch to the matching language as well.
- Testing: type checking guarantees that every language pack has exactly the same keys as Simplified Chinese; browser tests cover switching languages, persistence after refresh, `<html lang>`, and that the mobile layout does not overflow (Russian text is longer).

## 2. Software store

### Catalog

- The built-in catalog is packaged into the backend with Go `embed`, one manifest per software: `id`, version, translation keys for the name and description, protocol category, required ports, setting definitions (type, default, allowed range, whether sensitive), and the image pinned by digest.
- The first catalog (consistent with the entry protocols in the node design):

| id          | Software                            | Description                                                                   |
| ----------- | ----------------------------------- | ----------------------------------------------------------------------------- |
| `xray`      | Xray-core                           | Two kinds of entry points: VLESS + REALITY + Vision (TCP) and XHTTP + REALITY |
| `hysteria2` | Hysteria 2 (official server)        | UDP, masquerade or Salamander obfuscation                                     |
| `amneziawg` | AmneziaWG (`docker/node/amneziawg`) | UDP, 2.0 obfuscation parameters generated randomly per node                   |

### Installation state and API

- A new table `installed_apps(app_id, version, settings, installed_at, updated_at)`; no settings are stored in the database for software that is not installed.
- `GET /api/apps`: the catalog plus installation state. `POST /api/apps/:id/install`, `POST /api/apps/:id/uninstall`, `GET/PUT /api/apps/:id/settings`. Installing, uninstalling, and changing settings require a session and the same-origin rules; uninstalling requires entering the password again.
- Settings are validated against the manifest (unknown fields and out-of-range values are rejected). Sensitive settings are stored encrypted with `master.key`, and reads do not return the plaintext.
- Uninstalling deletes the software's settings; if it is deployed to nodes, the response is `409 APP_DEPLOYED` (no deployments can exist before node phase 2).

### Interface

- A new "Store" page: lists the software in the catalog with version, protocol, and installation state, and offers install and uninstall.
- A new "Installed software" group in the sidebar lists only installed software; each software has its own settings page and management page. Software that is not installed has no entry, and visiting its address directly shows "{name} isn’t installed" and points to the store.
- "Deploy to nodes" on the management page shows the real not-yet-available state until node phase 2 is complete, and displays no fictional data.
- The nodes page no longer lists "planned protocols"; it links to the software store instead.

## 3. Certificates and DNS providers

### Dependencies

- Use `github.com/go-acme/lego/v4` (a mature Go ACME client, pure Go, buildable without CGO), importing only the subpackages of the four DNS providers to avoid pulling in all of its dependencies.

### DNS provider credentials

- A new table `dns_credentials(id, name, provider, secrets, created_at, updated_at)`. `secrets` is encrypted as a whole with `master.key`; neither the API nor the logs return the plaintext, and the interface shows only whether it is set and when it was last changed.
- `name` is an optional note ("Note (optional)" in the interface), limited to 64 Unicode characters after trimming. Existing names remain notes and can be cleared. Credential lists, certificate selectors, and certificate lists share one display rule: show the note, or the localized provider name when empty. Empty notes remain empty in storage, and the credential list omits duplicate provider subtitles. Certificate responses include the current note and provider so subsequent queries reflect edits.
- Fields required by each provider: Cloudflare (API Token), Alibaba Cloud (AccessKey ID / AccessKey Secret), Tencent Cloud DNSPod (SecretId / SecretKey), AWS Route 53 (Access Key ID / Secret Access Key / region, optional Hosted Zone ID).
- Adding, changing, and deleting credentials require entering the password again; credentials referenced by a certificate cannot be deleted (`409 CREDENTIAL_IN_USE`).
- A "Verify" action is provided: it calls the provider's read-only API to confirm that the credentials work, without creating any DNS records.

### ACME accounts and certificates

- ACME services: Let's Encrypt production (default) and staging are built in; a custom service takes a directory URL (only `https://` is accepted) and an optional EAB Key ID / HMAC.
- ACME account private keys are generated on the console and stored encrypted with `master.key`; each combination of ACME service and contact email corresponds to one account.
- A new table `certificates(id, domains, ca_directory, dns_credential_id, status, not_before, not_after, certificate_pem, private_key, last_error, renew_at, created_at)`. Certificate private keys are stored encrypted; the certificate chain can be downloaded, but the private key is not downloadable through the browser (node agents will pull it as needed later).
- Statuses: `pending` (queued/issuing), `valid`, `failed`, `expired`. Issuance runs in a backend background task and the API returns immediately; the interface polls for status.
- Automatic renewal: renewal starts 30 days before expiry; failures are retried with exponential backoff and the error is shown in the interface; the console checks at startup and once every hour.
- Domain validation: only valid DNS names are accepted, `*.example.com` wildcards are supported; a single certificate holds at most 100 names (the ACME limit).
- Logs record only the route template, method, status, and duration; issuance errors keep only the error category and a short description returned by ACME/DNS, never credentials or tokens.

### Testing

- Go tests use Let's Encrypt's Pebble (an ACME server for testing) and an in-process test DNS provider, without reaching the public internet or calling real providers.
- Credential encryption, API permissions, password confirmation, deletion restrictions, and renewal scheduling all have unit tests; browser tests cover the credential form not echoing plaintext, and the certificate list and status display.

## Implementation order

1. Localization (new text for the other two parts goes directly into the language packs).
2. The software store and certificate management can proceed in parallel: database migration versions are assigned in order (software store v3, certificates v4), and language packs are split into files by feature.
3. Each part comes with its own tests and documentation updates; the main session runs `make check` once before merging.
