# Certificate and DNS provider API contract

English | [简体中文](certs-api.zh-CN.md)

Corresponds to section 3, "Certificates and DNS providers", of the [platform design](../../../docs/platform-design.md). The console stores DNS provider credentials, requests certificates through the ACME protocol with DNS-01 validation, and renews them automatically in the background. The API is implemented by the Go backend in `app/backend/internal/certs`; the frontend client is `src/api/certs.ts`.

## Transport rules

Follows the [authentication contract](auth-api.md): same origin, `credentials: same-origin`, `X-Requested-With: Veilpost`; write requests accept only JSON and check `Origin` (in development mode, same-origin http/https; in production it must be HTTPS and match `VPN_PUBLIC_ORIGIN`); redirects are not followed; 10-second timeout; write operations are not retried automatically; all responses carry `Cache-Control: no-store`. Every endpoint requires a full session that has completed two-step verification; otherwise it returns `401 SESSION_EXPIRED`. A request body must be a single JSON object; unknown fields return `400 REQUEST_FAILED`.

## DNS provider credentials `/api/dns-credentials`

### Credential object

```ts
interface DnsCredential {
    id: string // 22-character base64url random identifier
    name: string
    provider: 'cloudflare' | 'alidns' | 'tencentcloud' | 'route53'
    fields: Record<string, boolean> // whether each field is saved; values are never returned
    certificates: number // number of certificates that reference this credential
    createdAt: string
    updatedAt: string
}
```

Fields per provider (the keys of `fields` are exactly all of that provider's fields):

| provider       | Fields                                                                               |
| -------------- | ------------------------------------------------------------------------------------ |
| `cloudflare`   | `apiToken`                                                                           |
| `alidns`       | `accessKeyId`, `accessKeySecret`                                                     |
| `tencentcloud` | `secretId`, `secretKey`                                                              |
| `route53`      | `accessKeyId`, `secretAccessKey`, `region`, `hostedZoneId` (the only optional field) |

Values are printable ASCII without whitespace, at most 512 bytes; `region` looks like `us-east-1`; `hostedZoneId` consists of uppercase letters and digits (optionally with a `/hostedzone/` prefix). All fields (including the region and Hosted Zone ID) are encrypted together as one unit with `master.key`; neither the API nor the logs return plaintext.

### Endpoints

| Method and path                        | JSON request body                       | Success response                       |
| -------------------------------------- | --------------------------------------- | -------------------------------------- |
| `GET /api/dns-credentials`             | —                                       | `200 { credentials: DnsCredential[] }` |
| `POST /api/dns-credentials`            | `{ name, provider, secrets, password }` | `201 { credential }`                   |
| `POST /api/dns-credentials/:id/update` | `{ name, secrets, password }`           | `200 { credential }`                   |
| `POST /api/dns-credentials/:id/delete` | `{ password }`                          | `204`                                  |
| `POST /api/dns-credentials/:id/verify` | `{}`                                    | `200 { status: 'ok' }`                 |

- Adding, changing, and deleting all require the current password and share the per-session rate limit (15 per 5 minutes) with account security operations. A wrong password returns `403 INVALID_PASSWORD` and changes no data.
- The provider cannot be changed on update. A non-empty value in `secrets` replaces that field, an empty string clears an optional field (an empty required field returns `INVALID_SECRETS`), and fields that are absent keep their existing values. The interface sends only the fields the user filled in and never reads saved values.
- A credential referenced by a certificate cannot be deleted: `409 CREDENTIAL_IN_USE`.
- Verifying a credential calls one read-only provider API: Cloudflare `GET /zones?per_page=1`, Alibaba Cloud `DescribeDomains` (1 item per page), Tencent Cloud `DescribeDomainList` (Limit 1), Route 53 `GetHostedZone` (when a Hosted Zone ID is filled in) or `ListHostedZones` (MaxItems 1). It does not create or modify any DNS records. Timeout is 8 seconds; per-session rate limit is 10 per 5 minutes. A provider rejection returns `422 CREDENTIAL_REJECTED`; a network error or provider failure returns `424 PROVIDER_UNREACHABLE`.

## Certificates `/api/certificates`

### Certificate object

```ts
interface Certificate {
    id: string
    domains: string[] // normalized names (lowercase, Punycode); the first is used for the download file name
    directory: 'letsencrypt' | 'letsencrypt-staging' | 'custom'
    directoryUrl: string // https:// ACME directory URL
    email: string // ACME account contact email, may be empty
    dnsCredentialId: string
    dnsCredentialName: string
    status: 'pending' | 'valid' | 'failed' | 'expired'
    issuing: boolean // an issuance or renewal task is queued or running in this process
    issuer: string | null // organization name of the leaf certificate's issuer (CN if none)
    notBefore: string | null
    notAfter: string | null
    renewAt: string | null // time of the next automatic issuance or retry
    lastError: { code: string; detail: string | null; at: string } | null
    createdAt: string
    updatedAt: string
}
```

- `pending`: first issuance queued or in progress. `valid`: issued and not expired. `failed`: first issuance failed; waiting for a manual reissue. `expired`: the certificate has expired (occurs when renewal keeps failing).
- During renewal the status stays `valid` and `issuing` is `true`; the interface shows "Renewing".
- `lastError.code` is the issuance error category (see the table below); `detail` is present only when the ACME service returned a problem document, and contains the problem type and a short description (from the CA, untranslated, at most 300 characters); the original text of DNS provider errors is never stored. It is cleared after a successful issuance.
- The certificate private key never appears in any API response.

| Issuance error category   | Meaning                                                                        |
| ------------------------- | ------------------------------------------------------------------------------ |
| `CREDENTIAL_UNAVAILABLE`  | The credential cannot be decrypted or the provider client cannot be built      |
| `ACME_UNREACHABLE`        | Cannot connect to the ACME directory, or a network error                       |
| `ACME_ACCOUNT_FAILED`     | ACME account registration failed (including EAB required but not provided)     |
| `ACME_RATE_LIMITED`       | ACME returned `rateLimited`                                                    |
| `ACME_REJECTED`           | ACME returned another problem (validation failure, rejected name, etc.)        |
| `DNS_PROVIDER_FAILED`     | Creating the validation TXT record through the provider failed                 |
| `DNS_PROPAGATION_TIMEOUT` | The validation record did not appear on the authoritative DNS within 5 minutes |
| `ISSUANCE_FAILED`         | Other errors                                                                   |

### Endpoints

| Method and path                      | JSON request body                                                                       | Success response                      |
| ------------------------------------ | --------------------------------------------------------------------------------------- | ------------------------------------- |
| `GET /api/certificates`              | —                                                                                       | `200 { certificates: Certificate[] }` |
| `POST /api/certificates`             | `{ domains, dnsCredentialId, directory, directoryUrl, email, eab: { keyId, hmacKey } }` | `202 { certificate }`                 |
| `POST /api/certificates/:id/reissue` | `{}`                                                                                    | `202 { certificate }`                 |
| `POST /api/certificates/:id/delete`  | `{}`                                                                                    | `204`                                 |
| `GET /api/certificates/:id/chain`    | —                                                                                       | `200` PEM certificate chain           |

- A request returns `pending` immediately and issuance runs in the background; the interface refreshes every 3 seconds while any certificate is `pending` or `issuing`, otherwise every 30 seconds. Requesting and reissuing share a per-session rate limit (20 per 10 minutes).
- `domains`: 1–100 valid DNS names, each with at least two levels, labels of 1–63 letters, digits, or hyphens, and a total length of at most 253; a name may start with `*.` to denote a wildcard (allowed only as the leftmost level); Unicode names are converted to Punycode; duplicates are removed. Invalid names return `400 INVALID_DOMAIN`; more than 100 return `400 TOO_MANY_DOMAINS`.
- `directory`: `letsencrypt` (the default; may also be an empty string) is the Let's Encrypt production environment and `letsencrypt-staging` is the staging environment, in which case `directoryUrl` must be empty; with `custom`, `directoryUrl` must be an `https://` URL without user information or a fragment; otherwise `400 INVALID_DIRECTORY`.
- `email` may be empty; otherwise it must be a plain email address (`400 INVALID_EMAIL`). The same directory URL and email share one ACME account; the account private key (ECDSA P-256) is generated on the console and stored encrypted with `master.key`.
- `eab` can be used only with a custom directory; the Key ID and HMAC Key (base64url) must both be filled in, otherwise `400 INVALID_EAB`. EAB is stored encrypted until that account registers successfully and deleted after registration; an already registered account ignores newly submitted EAB.
- Submitting a request means agreeing to the terms of service of the selected ACME service (`termsOfServiceAgreed: true` at account registration).
- Credential not found: `404 CREDENTIAL_NOT_FOUND`. Certificate not found: `404 CERTIFICATE_NOT_FOUND`.
- Reissue: if a task is already in progress, the current state is returned directly; a `failed` certificate returns to `pending` and its error is cleared; a certificate that already has an issued certificate stays `valid`, and the certificate and private key are replaced after a successful issuance.
- Deletion removes the certificate and its encrypted private key and cancels any issuance in progress; the ACME account is kept for later reuse.
- Certificate chain: `Content-Type: application/pem-certificate-chain`, `Content-Disposition: attachment; filename=<first name>.pem` (`*.` is written as `wildcard.`); the content is a PEM chain with the leaf certificate first. If not yet issued, it returns `409 CERTIFICATE_NOT_ISSUED`. The client checks that the response contains only `CERTIFICATE` blocks and then downloads it through a temporary Blob. The private key is not offered for browser download; the node agent will fetch it on demand later.

## Background tasks

- Issuance and renewal run in backend tasks, at most 2 concurrently; the console checks at startup and every hour afterward: it marks `valid` certificates whose `notAfter` has passed as `expired`, and queues certificates whose `renewAt` has arrived.
- After a successful issuance, `renewAt` is 30 days before expiry; certificates valid for less than 90 days are renewed when one third of their validity remains. Every issuance generates a new certificate private key (ECDSA P-256).
- A failed renewal keeps the existing certificate and backs off at 1, 2, 4, 8, and 16 hours, then retries every 24 hours; a failed first issuance is not retried automatically and requires a manual reissue.
- When the console shuts down or a certificate is deleted, tasks in progress are canceled immediately: ACME requests are bound to the task context, and the DNS propagation wait exits at its next check. Interrupted tasks record no error and resume at the next startup.

## Error codes

| Error code                | Status | Meaning                                                                  |
| ------------------------- | ------ | ------------------------------------------------------------------------ |
| `INVALID_CREDENTIAL_NAME` | 400    | Name is empty, longer than 64 characters, or contains control characters |
| `INVALID_PROVIDER`        | 400    | Unsupported provider                                                     |
| `INVALID_SECRETS`         | 400    | Fields are missing, unknown, or malformed                                |
| `CREDENTIAL_NOT_FOUND`    | 404    | Credential does not exist                                                |
| `CREDENTIAL_IN_USE`       | 409    | Credential is referenced by a certificate and cannot be deleted          |
| `CREDENTIAL_UNREADABLE`   | 409    | The saved credential cannot be decrypted (must be re-entered)            |
| `CREDENTIAL_REJECTED`     | 422    | The provider rejected the credential during verification                 |
| `PROVIDER_UNREACHABLE`    | 424    | Cannot connect to the provider during verification                       |
| `INVALID_DOMAIN`          | 400    | Invalid domain name                                                      |
| `TOO_MANY_DOMAINS`        | 400    | More than 100 names                                                      |
| `INVALID_DIRECTORY`       | 400    | Invalid ACME service or directory URL                                    |
| `INVALID_EMAIL`           | 400    | Invalid contact email                                                    |
| `INVALID_EAB`             | 400    | EAB is incomplete, malformed, or used with a built-in directory          |
| `CERTIFICATE_NOT_FOUND`   | 404    | Certificate does not exist                                               |
| `CERTIFICATE_NOT_ISSUED`  | 409    | Certificate not yet issued; the chain cannot be downloaded               |

The generic `SESSION_EXPIRED`, `INVALID_PASSWORD`, `TOO_MANY_ATTEMPTS`, `REQUEST_FAILED`, and `SERVICE_UNAVAILABLE` also apply. Every error code has text in all four languages' `src/locales/<locale>/errors.ts`; the text for issuance error categories is in `certificates.issueErrors` in `certs.ts`. The client treats a 404 with an error code ending in `_NOT_FOUND` as a missing resource, and any other 404 as the service being unavailable.
