# Software store API contract

English | [简体中文](apps-api.zh-CN.md)

Corresponds to section 2, "Software store", of the [platform design](../../../docs/platform-design.md). "Installing" a VPN software package in the software store only enables it in the console: its settings and management page appears in the sidebar, and its settings are stored in the console database. Deploying to nodes depends on phase 2 of the [node design](../../../docs/nodes-design.md) and is not implemented yet.

The API is implemented by the Go backend in `app/backend/internal/apps`; the client is `src/api/apps.ts`.

## Built-in catalog

- The catalog is packaged into the backend binary with Go `embed` (`internal/apps/catalog/<id>.json`) and released with the console version; remote software sources and uploading custom packages are not supported. At startup the backend strictly validates every manifest (unknown fields, invalid defaults, images without a pinned digest, and similar problems all make startup fail).
- Names, descriptions, setting labels, hints, and enum options in a manifest are all language pack keys and must be under `apps.catalog.<id>.`; the backend returns no interface text. `tests/dev/locales.test.ts` checks that every key referenced by a manifest exists in the language packs.
- First catalog version:

| id          | Version        | Protocols (transport)                                         | Image                                                                                                                                 |
| ----------- | -------------- | ------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `xray`      | `26.11.9`      | VLESS + REALITY + Vision (TCP), VLESS + XHTTP + REALITY (TCP) | `ghcr.io/xtls/xray-core:26.11.9@sha256:9812e382e229e9ed63fd179bf1a4e3fe244c12ff3ae4fd8128745d79c87575ab`                              |
| `hysteria2` | `2.14.0`       | Hysteria 2 (UDP)                                              | `docker.io/tobyxdd/hysteria:v2.14.0@sha256:b15cedc3de200ae7a4f177908a83ef23ed3de2935458b22b7d46a7a8697038a9`                          |
| `amneziawg` | `3.1.20261116` | AmneziaWG 2.0 (UDP)                                           | Built from `docker/node/amneziawg` in this repository, no published location yet (`image.build`, with no `reference` and no `digest`) |

Published images are pinned by multi-architecture index (OCI image index) digest; each digest was double-checked against the registry's `Docker-Content-Digest` response header and the SHA-256 of the manifest content (2026-11-23). The Xray image is pushed to `ghcr.io/xtls/xray-core` by the release workflow of the upstream repository `XTLS/Xray-core`; the official Hysteria 2 image is pushed to `tobyxdd/hysteria` on Docker Hub by the release workflow of `apernet/hysteria`. When upgrading, the version, tag, and digest must be changed together.

## Transport rules

The transport rules of the [authentication contract](auth-api.md) apply: same-origin, `credentials: same-origin`, `X-Requested-With: Veilpost`; write requests accept only JSON and have their `Origin` checked; redirects are not followed; 10-second timeout; write operations are not retried automatically; all responses set `Cache-Control: no-store`. Every endpoint requires a full session with two-step verification completed, and otherwise returns `401 SESSION_EXPIRED`. All write requests use `POST`, consistent with the authentication and node APIs.

## Objects

```ts
interface StoreApp {
    id: string // ^[a-z][a-z0-9-]{1,31}$
    version: string // version in the catalog
    name: string // language pack key
    description: string // language pack key
    protocols: { id: string; name: string; transport: 'tcp' | 'udp' }[] // name is a proper noun and is not translated
    ports: {
        setting: string // the corresponding port setting
        protocol: string
        transport: 'tcp' | 'udp'
        port: number | null // installed: current setting; not installed: default; null means chosen randomly per node at deployment
    }[]
    settings: AppSetting[]
    image: {
        reference: string // repository:tag@digest; empty string for a local build
        repository?: string
        tag?: string
        digest?: string // sha256:<64 hex digits>, multi-architecture index digest
        platforms?: string[] // e.g. linux/amd64
        build?: string // build directory in this repository, e.g. docker/node/amneziawg
        source: string // upstream source repository
    }
    installed: null | {
        version: string // catalog version at install time
        installedAt: string
        updatedAt: string
        configured: boolean // every required setting has a value
    }
}

interface AppSetting {
    key: string // ^[a-z][A-Za-z0-9]{0,31}$
    type:
        | 'port'
        | 'integer'
        | 'string'
        | 'enum'
        | 'boolean'
        | 'hostname'
        | 'hostport'
        | 'url'
    label: string // language pack key
    hint?: string // language pack key
    required?: true
    requiredWhen?: { setting: string; equals: string } // required when another enum setting has this value
    default?: number | string | boolean
    min?: number // integer
    max?: number // integer
    minLength?: number // string
    maxLength?: number // string
    pattern?: string // string, anchored regular expression
    options?: { value: string; label: string }[] // enum
    sensitive?: true // write-only, only for a string without a default
}
```

Setting value validation (the server is authoritative):

| Type       | Accepted values                                                                                                          |
| ---------- | ------------------------------------------------------------------------------------------------------------------------ |
| `port`     | An integer from 1 to 65535                                                                                               |
| `integer`  | An integer from `min` to `max`                                                                                           |
| `string`   | Between `minLength` and `maxLength` characters, no control characters, and matching `pattern` (if any)                   |
| `enum`     | One of the `value`s in `options`                                                                                         |
| `boolean`  | `true` / `false`                                                                                                         |
| `hostname` | A DNS name with at least two labels, converted to lowercase; IP addresses are not accepted                               |
| `hostport` | `hostname` or `hostname:port`, stored as `hostname:port` (443 when the port is omitted)                                  |
| `url`      | `https://` plus a DNS name, optionally with a port and path; user info, query parameters, and fragments are not accepted |

## Endpoints

| Method and path                | JSON request body       | Success response           |
| ------------------------------ | ----------------------- | -------------------------- |
| `GET /api/apps`                | —                       | `200 { apps: StoreApp[] }` |
| `POST /api/apps/:id/install`   | `{}`                    | `200 { app: StoreApp }`    |
| `POST /api/apps/:id/uninstall` | `{ password }`          | `204` (no response body)   |
| `GET /api/apps/:id/settings`   | —                       | `200 AppSettings`          |
| `POST /api/apps/:id/settings`  | `{ values?, secrets? }` | `200 AppSettings`          |

```ts
interface AppSettings {
    values: Record<string, number | string | boolean | null> // every non-sensitive setting; null when not set
    secrets: Record<string, boolean> // whether each sensitive setting is set; never contains values
    configured: boolean
    updatedAt: string
}
```

- The list returns all software in catalog order, with installation state. Reading the list writes no data.
- Install: creates the settings from the manifest defaults; required settings without a default (for example the site that REALITY borrows) stay unset, and `configured` is `false`. Software not in the catalog returns `404 APP_NOT_FOUND`; software that is already installed returns `409 APP_ALREADY_INSTALLED`.
- Uninstall: requires re-entering the current password (sharing the password confirmation rate limit of "15 per session per 5 minutes" with account security and node operations); a wrong password returns `403 INVALID_PASSWORD`, and exceeding the rate limit returns `429 TOO_MANY_ATTEMPTS`. On success, all settings of the software are deleted, including the encrypted sensitive values; reinstalling starts from the defaults. Software that is not installed returns `404 APP_NOT_INSTALLED`. Software already deployed to nodes returns `409 APP_DEPLOYED`; there is no deployment before node phase 2, so this does not occur yet, and the check stays inside the uninstall transaction.
- Read settings: software that is not installed returns `404 APP_NOT_INSTALLED`. Sensitive settings only report whether they are set.
- Change settings:
    - `values` replaces the non-sensitive settings as a whole: settings that are omitted or `null` return to their default, or become unset if they have no default.
    - `secrets` is write-only: omitted keys keep their saved value, a string replaces it, and `null` clears it.
    - A sensitive setting in `values`, a non-sensitive setting in `secrets`, or a key not in the manifest: `400 { code: 'UNKNOWN_SETTING' }`.
    - An invalid value: `400 { code: 'INVALID_SETTING', field }`; a required setting (including one made required by `requiredWhen`) still without a value after saving: `400 { code: 'SETTING_REQUIRED', field }`. `field` is the setting key from the manifest and contains no value.
    - Software that is not installed returns `404 APP_NOT_INSTALLED`.
- Unknown fields, multiple JSON values, or a request body over 16 KiB return `400 REQUEST_FAILED`; a missing `Origin`, custom request header, or JSON content type returns `403 REQUEST_FAILED`.

## Storage and security

- Table `installed_apps(app_id PRIMARY KEY, version, settings, installed_at, updated_at)` (database migration v3). No settings are stored for software that is not installed.
- `settings` is JSON: `{ values: {...}, secrets: { key: <ciphertext> } }`. Sensitive values are encrypted with `master.key` (AES-256-GCM, random nonce), with additional data bound to `apps:<id>:<key>`, so a ciphertext cannot be moved to another software package or setting; no endpoint returns the plaintext.
- On read, saved values are revalidated against the current manifest; after a console upgrade, values the manifest no longer accepts are treated as unset.
- Logs record only the route template, method, status, and duration, and never the software identifier, setting values, or passwords.

## Frontend requirements

- The store page (`/store`) lists the software in the catalog with its version, protocols, ports, image, and installation state, and offers install and uninstall (uninstalling requires the current password).
- The "Installed software" group in the sidebar lists only installed software, with one page per software package (`/apps/:id`). The bottom navigation on narrow screens keeps four items; the store entry and installed software appear in a row below the brand.
- A software page generates its settings form from the manifest and shows the server's field-level errors after submitting. Sensitive fields are write-only: they show "Set / Not set", leaving the input empty keeps the saved value, and the value can be cleared; input values are kept only in component memory, are cleared after a successful save, when leaving the page, or when the session ends, and never enter browser storage, URLs, logs, or persisted Pinia stores.
- "Deploy to nodes" in the management area shows its real unavailable state until node phase 2, and does not show fictional nodes or deployment data.
- Visiting the address of software that is not installed or not in the catalog shows "{name} isn’t installed" or "This software isn’t in the catalog", and guides the user to the store.
- The catalog and installation state are kept in memory in `src/stores/apps.ts`, are not persisted, and are cleared on sign-out.
- The frontend only controls presentation; sessions, write request rules, settings validation, password confirmation, and rate limiting are all enforced by the backend.
