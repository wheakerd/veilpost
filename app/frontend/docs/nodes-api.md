# Node and agent API contract

English | [简体中文](nodes-api.zh-CN.md)

Corresponds to phase 1 of the [node design](../../../docs/nodes-design.md): node enrollment and the control channel. The console only registers nodes, issues single-use enrollment tokens, verifies the agent's signed requests, and records online status; this phase does not deliver VPN configuration and does not manage any containers.

The API is implemented by the Go backend (`app/backend/internal/nodes`); the agent is `veilpost-agent` in `app/agent`. Both parts share the signature format below and pin the same known-answer signature vector in their respective Go tests.

## Browser API `/api/nodes`

Follows the transport rules of the [authentication contract](auth-api.md): same origin, `credentials: same-origin`, `X-Requested-With: Veilpost`; write requests accept only JSON and check `Origin`; redirects are not followed; 10-second timeout; write operations are not retried automatically; all responses carry `Cache-Control: no-store`. Every endpoint requires a full session that has completed two-step verification; otherwise it returns `401 SESSION_EXPIRED`.

### Node object

```ts
interface VpnNode {
    id: string // 22-character base64url random identifier
    name: string
    status: 'pending' | 'active' | 'revoked'
    online: boolean
    agentVersion: string | null
    createdAt: string
    enrolledAt: string | null
    revokedAt: string | null
    lastSeenAt: string | null
    enrollmentExpiresAt: string | null
}
```

- `pending`: created, not yet enrolled. `enrollmentExpiresAt` is the expiry time of a still-valid enrollment token; after the token expires or is cleaned up it is `null`, the interface shows "Token expired", and a new token can be issued.
- `active`: the agent has bound a public key using the enrollment token.
- `revoked`: revoked and cannot be restored; reconnecting requires creating a new node. Revoked nodes can be deleted.
- `online` is computed by the server: it is `true` only when `status` is `active` and the last verified signed request (`lastSeenAt`) was within 90 seconds. The agent starts the next long poll immediately after each one ends, so 90 seconds covers several polling cycles.
- All times are ISO 8601 strings with a time zone. Missing data is `null`, and the interface shows "Unknown" without filling in a default value.

### Endpoints

| Method and path                  | JSON request body    | Success response                                |
| -------------------------------- | -------------------- | ----------------------------------------------- |
| `GET /api/nodes`                 | —                    | `200 { nodes: VpnNode[] }`                      |
| `POST /api/nodes`                | `{ name, password }` | `201 { node: VpnNode, enrollment: Enrollment }` |
| `POST /api/nodes/:id/revoke`     | `{ password }`       | `200 { node: VpnNode }`                         |
| `POST /api/nodes/:id/enrollment` | `{ password }`       | `200 { node: VpnNode, enrollment: Enrollment }` |
| `POST /api/nodes/:id/delete`     | `{ password }`       | `204` (no response body)                        |

```ts
interface Enrollment {
    token: string // 43-character base64url, 256-bit random value
    expiresAt: string // 15 minutes after creation
}
```

- The list is ordered "non-revoked first, then by creation time descending" and contains no tokens or public keys.
- Create: after trimming leading and trailing whitespace, `name` must be 1–64 characters and must not contain control characters; otherwise `400 INVALID_NAME`. The current password must be re-entered; if it is wrong, `403 INVALID_PASSWORD`. The enrollment token appears only in this response; the database stores only its SHA-256 hash.
- Revoke: requires the current password. Revocation takes effect immediately: the node's signed requests are rejected, unused enrollment tokens and recorded nonces are deleted, and any waiting long poll ends immediately with `NODE_REVOKED`. Revoking an already revoked node again returns that node (idempotent). A nonexistent node returns `404 NODE_NOT_FOUND`.
- Reissue: applies only to `pending` nodes and requires the current password. All previous unused enrollment tokens of the node are invalidated immediately; the new token is valid for 15 minutes and appears only in this response. A node that is already enrolled or revoked returns `409 NODE_NOT_PENDING`.
- Delete: applies only to `revoked` nodes and requires the current password. The node record, its enrollment tokens, and its nonce records are deleted together and cannot be restored. A node that is not revoked returns `409 NODE_NOT_REVOKED`; a nonexistent node returns `404 NODE_NOT_FOUND`.
- Create, revoke, reissue, and delete share the password-confirmation rate limit of "15 per session per 5 minutes" with account security operations; exceeding it returns `429 TOO_MANY_ATTEMPTS`.
- Unknown fields, multiple JSON values, or a request body larger than 8 KiB return `400 REQUEST_FAILED`; a missing `Origin`, custom request header, or JSON content type returns `403 REQUEST_FAILED`.

### Frontend requirements

- The nodes page shows the real list; when there are no nodes it keeps the "None connected" empty state and shows no sample nodes. While the page is visible it refreshes in the background every 15 seconds (without showing the global request bar); when a refresh fails it keeps the last real result and shows a notice.
- The enrollment token is kept only in the page component's memory: the enrollment command and token are shown with copy buttons; the token is cleared immediately when "Done, hide token" is clicked, the token expires, the user leaves the page, or the session expires. Tokens and passwords must not enter localStorage, sessionStorage, URLs, logs, or persisted Pinia stores.
- The enrollment command uses the current page's Origin: `veilpost-agent enroll --server <origin> --token-stdin`. The user pastes the token after the command prompts for it; it is not written into command-line arguments. If the address the agent uses to reach the console differs from the browser's, the user needs to replace `--server`.
- The frontend only controls presentation; sessions, passwords, rate limiting, and revocation are all enforced by the backend.

## Agent API `/api/agent`

The agent does not use cookies, and `Origin` is not checked. Request bodies are JSON (`Content-Type: application/json`, at most 64 KiB; otherwise `415` / `413 REQUEST_FAILED`). For compatibility across versions, the agent API ignores unknown fields in the request body.

### Enroll `POST /api/agent/enroll`

```ts
// Request
{
    token: string // single-use enrollment token
    publicKey: string // Ed25519 public key, 32 bytes, base64url (no padding)
    proof: string // signature of the enrollment message with the corresponding private key, 64 bytes, base64url
    agentVersion: string // ^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$
}
// 200 response
{
    nodeId: string
    name: string
}
```

Enrollment message (joined with `\n`, no trailing newline):

```text
vpn-agent-enroll-v1
<SHA-256 hex of the token>
<publicKey as sent>
```

In a single transaction, the server: confirms that the token hash exists, is unused, has not expired, and that the node is still `pending`; marks the token as used and deletes the node's other unused tokens; writes the public key, agent version, and enrollment time, and sets the node to `active`. The same token can succeed only once (even among concurrent requests, only one succeeds).

| Case                                                           | Response                 |
| -------------------------------------------------------------- | ------------------------ |
| Malformed field or `proof` verification failure                | `400 REQUEST_FAILED`     |
| Token does not exist, is used, has expired, or node revoked    | `401 INVALID_ENROLLMENT` |
| Public key already used by another node                        | `409 KEY_IN_USE`         |
| More than 30 enrollments per 5 minutes from one client address | `429 TOO_MANY_ATTEMPTS`  |

Client addresses are identified with the same rules as login: `X-Forwarded-For` is trusted only from proxies listed in `VPN_TRUSTED_PROXIES`.

### Signed requests

Apart from enrollment, every agent request carries:

| Request header    | Content                                                                 |
| ----------------- | ----------------------------------------------------------------------- |
| `X-VPN-Node`      | Node ID                                                                 |
| `X-VPN-Timestamp` | Unix seconds, decimal                                                   |
| `X-VPN-Nonce`     | 22–64-character base64url nonce (the agent uses a 24-byte random value) |
| `X-VPN-Signature` | Ed25519 signature, 64 bytes, base64url                                  |

Signed message (joined with `\n`, no trailing newline):

```text
vpn-agent-v1
<HTTP method, e.g. POST>
<API path, e.g. /api/agent/poll>
<node ID>
<timestamp>
<nonce>
<SHA-256 hex of the raw request body bytes>
```

The path is the API's own path (without reverse proxy prefixes or query parameters), so the console can be deployed behind a proxy with a path prefix. The signature covers the method and path, which prevents reusing a signature for one endpoint on another.

Server check order:

1. Request header format; a timestamp that differs from server time by more than 60 seconds returns `401 CLOCK_SKEW`.
2. The node exists and has a bound public key, and the signature is valid; otherwise `401 AGENT_UNAUTHORIZED` (no distinction between a nonexistent node, an unenrolled node, or a wrong signature).
3. A revoked node returns `403 NODE_REVOKED`. Only requests with a valid signature can learn the revocation status.
4. Nonce deduplication: a node's nonce can be used only once within its timestamp's validity window (timestamp + 61 seconds); a replay returns `401 AGENT_UNAUTHORIZED`. Nonces are stored in the `agent_nonces` table and deleted by periodic cleanup after they expire.
5. Per-node rate limiting: 120 verified requests per node per 5 minutes; exceeding it returns `429 TOO_MANY_ATTEMPTS`. This quota is independent of the login and account security rate limits; requests with invalid signatures do not consume the node's quota.

### Long poll `POST /api/agent/poll` (signed)

```ts
// Request
{
    agentVersion: string
    configVersion: number
}
// 200 response
{
    configVersion: number
}
```

The server first records `lastSeenAt` (the "time of the last signed request") and `agentVersion`. If the `configVersion` in the request differs from the console's, it returns the current version immediately; otherwise it waits up to 25 seconds, returns `403 NODE_REVOKED` if the node is revoked during that time, and returns early when the service stops. In phase 1 the configuration version is always `0`; when the agent receives a different version it only logs it and does not apply any configuration.

Long-poll requests have a separately extended server write timeout; when deployed behind a reverse proxy or CDN, the read timeout must be greater than 25 seconds (Nginx defaults to 60 seconds).

## Data and logs

- `nodes`: name, status, Ed25519 public key (unique), agent version, configuration version, and creation / enrollment / revocation / last contact times.
- `node_enrollments`: token SHA-256 hash, node, expiry time, and time of use. Expired records (used or not) are deleted by the cleanup task that runs when the backend starts and every hour.
- `agent_nonces`: node, nonce, and expiry time, also deleted by the cleanup task.
- Node private keys exist only on the nodes; the console neither stores nor generates node private keys.
- Logs record only the route template (for example `/api/nodes/:id/revoke`), method, status, and duration; they do not record node IDs, tokens, public keys, signatures, request bodies, or passwords.

## Agent

- `veilpost-agent enroll --server <url> --token-stdin [--state-dir DIR]`: reads the token from standard input, generates `identity.key` (PKCS#8 PEM, `0600`) in the state directory (default `/var/lib/veilpost-agent`, permissions `0700`), and after successful enrollment writes `node.json` (server address, node ID, name, `0600`). It refuses to overwrite an existing identity; if enrollment fails, it deletes the newly generated private key. The server address must be HTTPS; only loopback addresses may use HTTP.
- `veilpost-agent run [--state-dir DIR]`: refuses a private key or state directory accessible to the group or other users; long-polls continuously and reports its version. On failure it backs off exponentially from 5 seconds up to 5 minutes (±20% jitter); clock skew produces a hint to synchronize time; when the node is revoked it exits with exit code 3, so the service manager can stop restarting it.
- The agent does not follow redirects and does not log tokens or private keys.

## Verification

- Go tests (`app/backend/tests/nodes`, `app/agent/tests/agent`) cover the known-answer signature vector, replay, clock skew, request tampering, unknown nodes, revocation and long-poll wake-up, single-use token redemption and expiry, concurrent redemption, public key reuse, per-node rate limiting, cleanup, HTTP write-request checks, and logging boundaries.
- The browser test `tests/e2e/nodes.spec.ts` creates and revokes nodes against the real backend, and within the test uses Node's Ed25519 implementation to call the real enrollment and signed poll endpoints, verifying online status and the effect of revocation; it does not run the `veilpost-agent` program. As decided, end-to-end integration tests between the agent and the console are not implemented for now.
