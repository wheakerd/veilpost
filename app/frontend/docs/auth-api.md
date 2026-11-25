# Sign-in and two-step verification API contract

English | [简体中文](auth-api.zh-CN.md)

This is currently implemented with Go + Gin + SQLite, and the endpoints below are served by the real backend. Neither development nor production has mock authentication or a default account; administrators are created with the local `make admin-create` command. The Docker Compose frontend connects to the backend through a same-origin proxy.

## Pages and state

- `/login`: username and password sign-in; when no administrator exists, it shows the local initialization command.
- `/login/verify`: verification with an authenticator code or a recovery code; the user can go back to username sign-in.
- `/settings/security`: authenticator binding, saving recovery codes, regenerating recovery codes, turning off two-step verification, changing the password, and signing out other sessions.
- Open the account center from the top bar to view your username, manage sign-in security and sessions, or sign out.
- All other console routes require a full session; while two-step verification is incomplete, they always return to the verification page.
- Navigation reads the session from the server; if a signed-in session was confirmed within the last 15 seconds, that read is skipped to avoid repeated requests when switching pages quickly. After a refresh, state is always restored from the server. Access is not granted when the authentication service is unavailable, returns HTML, or returns JSON that does not match the contract.
- The post-login return address may only be a known console page, and keeps its query parameters and fragment. External addresses and redirect loops back to the sign-in page fall back to `/`.

The frontend route guards only control the interface; they are not a backend authorization mechanism. All future management endpoints must still check the full session and permissions on the server.

## Transport and sessions

The API base path is fixed to the same-origin `/api/auth`. GET has no request body; POST uses JSON. The client sends `credentials: same-origin`, `Accept: application/json`, and `X-Requested-With: Veilpost`, does not follow API redirects, uses a 10-second timeout, and does not automatically retry write operations.

The backend stores a random session identifier in the `vpn_session` cookie with the attributes `HttpOnly; SameSite=Strict; Path=/`; production mode additionally enforces `Secure` and requires HTTPS. Only local HTTP development with an explicit `VPN_ENV=development` allows a non-Secure cookie. A cookie for which two-step verification is not complete must not authorize any management endpoint. The backend rotates the corresponding session on password sign-in and when verification completes, destroys the full session and any pending verification state on sign-out, and returns the anonymous state for an expired challenge.

For write operations, the backend checks the request `Origin` and the custom request header above, accepts only JSON, and must not enable credentialed CORS for untrusted origins; if a deployment uses a different CSRF scheme, the client should be adjusted to match. All authentication responses set `Cache-Control: no-store`. The frontend does not store passwords, verification codes, recovery codes, binding secrets, or session tokens in localStorage, sessionStorage, URLs, or persisted Pinia stores.

## Session response

`GET /session` returns one of the following three JSON shapes:

```ts
type AuthSession =
    | { status: 'anonymous'; setupRequired?: boolean }
    | { status: 'mfa_required'; expiresAt: string }
    | {
          status: 'authenticated'
          user: { id: string; username: string; twoFactorEnabled: boolean }
      }
```

`expiresAt` is an ISO 8601 time with a time zone. `401 { "code": "SESSION_EXPIRED" }` is also treated as anonymous. This endpoint must be able to restore the pending verification state from a short-lived cookie, without relying on the frontend to store a challenge identifier. After a full session expires, the old user information must no longer be returned.

## Sign-in

| Method and path | JSON request body                        | Success response                                   |
| --------------- | ---------------------------------------- | -------------------------------------------------- |
| `POST /login`   | `{ username, password }`                 | An authenticated session, or an `mfa_required` one |
| `POST /verify`  | `{ method: 'totp' \| 'recovery', code }` | An authenticated session                           |
| `POST /logout`  | `{}`                                     | `204`, and all session cookies are cleared         |

`username` has leading and trailing spaces removed; the password is kept as is. An authenticator code is a six-digit string that keeps leading zeros; a recovery code is a string. The frontend only removes whitespace from verification codes and recovery codes. A wrong password always returns `INVALID_CREDENTIALS`, so the response does not reveal whether the account exists.

For an account with two-step verification enabled, passing the password check can only create a short-lived challenge, never a full session ahead of time. The challenge is valid for five minutes; the server is responsible for expiry, attempt limits, authenticator code replay protection, and single-use consumption of recovery codes. The frontend shows a countdown, but the server's time and checks are authoritative. Signing in with a recovery code does not automatically turn off two-step verification.

## Enabling two-step verification

1. `POST /two-factor/setup`, with the request `{ password }`. Requires a full sign-in and the current password. Returns:

    ```ts
    {
      setupId: string
      secret: string
      expiresAt: string
      qrCodeDataUrl?: string
    }
    ```

    `secret` is uppercase Base32 (A–Z, 2–7, 16–128 characters). The backend generates a new random secret and stores a pending binding state; this step cannot enable two-step verification. The parameters are TOTP, SHA-1, 6 digits, and 30 seconds, which makes manual entry easy.

    The current backend always returns `qrCodeDataUrl`, and only `data:image/png;base64,...` is accepted. The QR code is generated locally on the backend by the Go TOTP library; the PNG includes a white quiet zone, and its content is the corresponding `otpauth://totp/` configuration, with the issuer `Veilpost`, the username as the account, and the same parameters as manual setup. External QR code URLs are not accepted, and the secret is never sent to third parties. The interface shows QR code binding first, with the manual secret collapsed by default; for compatibility, the client falls back to manual entry when an older backend omits the QR code or the image fails to load. On cancel, leaving the page, expiry, or successful binding, both the QR code and the temporary secret are cleared.

    `setupId` is bound to the current user, session, and validity period, and is valid for five minutes; a new setup request replaces the previous pending binding state. When the user cancels, navigates away, or refreshes, the frontend discards the temporary secret; the server lets an incomplete binding expire and must not enable it automatically.

2. `POST /two-factor/enable`, with the request `{ setupId, code }`. The server verifies the binding state and the first authenticator code, and on success atomically enables two-step verification and returns:

    ```ts
    {
      status: 'authenticated'
      user: { id: string; username: string; twoFactorEnabled: true }
      recoveryCodes: string[]
    }
    ```

    `recoveryCodes` is a non-empty list; the server generates high-entropy random recovery codes and stores only their irreversible hashes. The plaintext is returned only in this response; the frontend shows it on the current page and requires the user to confirm they have saved the codes before setup finishes. Leaving the page without saving shows a warning; once the page is canceled or the browser is closed, the plaintext cannot be recovered. The frontend does not generate secrets or recovery codes and does not validate the TOTP algorithm.

## Turning off two-step verification

`POST /two-factor/disable`, with the request `{ password, code }`, requires both the current password and an authenticator code; on success it returns a session with `status: 'authenticated'` and `user.twoFactorEnabled: false`. The server atomically removes the TOTP configuration, unused recovery codes, and the pending binding state, and revokes other sessions according to the service policy. The page updates its state only after a successful response.

## Recovery codes and security status

`GET /security` requires a full session and returns:

```ts
{
    recoveryCodesRemaining: number
    otherSessions: number
}
```

Both are non-negative integers. `otherSessions` counts only the unexpired full sessions of the same account other than the current session.

`POST /two-factor/recovery-codes`, with the request `{ password, code }`, requires two-step verification to be enabled, the current password, and an authenticator code (with the same replay protection). On success it returns `{ recoveryCodes: string[] }` and atomically replaces all unused recovery codes; the plaintext is returned only in this response, and the frontend reuses the save confirmation flow from enabling.

`POST /sessions/revoke-others`, with the request `{}`, returns `204` on success and destroys all sessions of the same account other than the current session (including pending verification challenges).

## Changing the password

`POST /password`, with the request `{ password, newPassword }`, requires a full session and the current password. The new password follows the creation rules (at least 12 characters and at most 256 UTF-8 bytes) and must differ from the current password. On success it returns an authenticated session; the server rotates the current session token and revokes the account's other sessions. The frontend requires the new password to be entered identically twice, and clears all password fields after submitting.

This version does not include self-service password recovery, account registration, or sending SMS or email. When both the authenticator and the recovery codes are lost, or the password is forgotten, an administrator with access to the server's data directory runs `make admin-reset-two-factor` or `make admin-reset-password` (in release packages, `admin-create.sh` with `ADMIN_ACTION=reset-two-factor` / `reset-password`); both revoke all of the account's sessions.

## Error format

Errors use a non-2xx HTTP status and `{ "code": "..." }`. The backend returns only the error code; the frontend shows the fixed text with the same name from `src/locales/<locale>/errors.ts` in the current interface language (shared by authentication and node error codes), and never shows backend error text directly. An error code missing from the language pack is shown with the text for `REQUEST_FAILED`. When adding an error code, it must be added to `errors.ts` in all four languages at the same time.

| code                                     | Suggested HTTP status | Frontend behavior                                                |
| ---------------------------------------- | --------------------- | ---------------------------------------------------------------- |
| `INVALID_CREDENTIALS`                    | 401                   | Keep the username, clear the password, allow retry               |
| `INVALID_PASSWORD`                       | 403                   | Stay on the security settings step, re-enter the password        |
| `WEAK_PASSWORD` / `PASSWORD_REUSED`      | 400                   | Keep the change password form, re-enter the new password         |
| `INVALID_CODE` / `INVALID_RECOVERY_CODE` | 400                   | Stay on the verification step, clear the input                   |
| `CHALLENGE_EXPIRED`                      | 401                   | Return to username sign-in                                       |
| `SESSION_EXPIRED`                        | 401                   | Clear the frontend session, sign in again                        |
| `SETUP_EXPIRED`                          | 410                   | Clear the pending binding secret, allow setting up again         |
| `TOO_MANY_ATTEMPTS`                      | 429                   | Ask the user to try again later; the server enforces rate limits |
| Other errors                             | 4xx / 5xx             | Report the failure, do not assume the operation succeeded        |

`404`, 5xx, network failures, timeouts, HTML fallbacks, and success responses that do not match the contract are all shown as the authentication service being unavailable. If sign-out fails, the current interface is kept and an error is shown, without claiming that the server session was destroyed.

## Integration and verification

The root `make start` uses Compose to start the real frontend and backend. Containers reach each other at `backend:8080`; the frontend proxy keeps the original Host, so browser requests remain same-origin. `VPN_PUBLIC_ORIGIN` can additionally bind an exact external Origin; production deployments must be configured with HTTPS.

A full session is valid for eight hours; pending verification and pending binding for five minutes. Passwords use Argon2id (64 MiB, 3 iterations, parallelism 2), TOTP secrets are encrypted with AES-GCM, and only hashes of recovery codes and session tokens are stored. TOTP accepts one adjacent time step and records used steps; enabling and turning off two-step verification and changing the password all revoke old sessions.

Sign-in is rate limited by client address (240 per 5 minutes), by account and address combined (10), and by account (100), so a single source cannot lock out an account by entering wrong passwords. Authenticator code and recovery code verification is rate limited per challenge (10 per 5 minutes) and per account (30 per hour), and signing in again does not reset the account budget. Security settings operations are rate limited per session (15 per 5 minutes). The client address is taken from `X-Forwarded-For` only when it is forwarded by a reverse proxy listed in `VPN_TRUSTED_PROXIES`.

When no user exists, the anonymous session includes `setupRequired: true`. This flag only guides local initialization; it does not open a registration or authentication bypass endpoint. Usernames are 3–64 ASCII letters, digits, dots, underscores, or hyphens; a password at creation is at least 12 characters and at most 256 UTF-8 bytes.

Browser tests live in `tests/e2e/` and use the real Go backend with a temporary SQLite database. `real-auth.ts` only creates isolated test accounts, calls real endpoints, and generates real TOTP codes; it does not intercept or fake HTTP responses. Go tests additionally cover concurrent recovery code consumption on the server, replay protection, rate limiting, and Origin and cookie boundaries. Tests do not access the development data volume.

References: [RFC 6238](https://datatracker.ietf.org/doc/html/rfc6238), [OWASP Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html).
