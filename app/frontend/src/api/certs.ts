import { authActivity } from './activity'
import {
    AuthApiError,
    apiRequest,
    invalidResponse,
    isDate,
    isRecord,
} from './auth'

// Contract: docs/certs-api.md. DNS provider secrets, EAB keys and the current
// password are write-only: they are sent once and kept only in component
// memory, never in storage, URLs or logs. Responses never contain them.

export const dnsProviders = [
    'cloudflare',
    'alidns',
    'tencentcloud',
    'route53',
] as const
export type DnsProvider = (typeof dnsProviders)[number]

export type SecretField =
    | 'apiToken'
    | 'accessKeyId'
    | 'accessKeySecret'
    | 'secretId'
    | 'secretKey'
    | 'secretAccessKey'
    | 'region'
    | 'hostedZoneId'

export interface ProviderField {
    name: SecretField
    required: boolean
    // Masked input for key material; identifiers and the region are plain text.
    masked: boolean
}

// Field definitions mirror app/backend/internal/certs/validate.go.
export const providerFields: Record<DnsProvider, readonly ProviderField[]> = {
    cloudflare: [{ name: 'apiToken', required: true, masked: true }],
    alidns: [
        { name: 'accessKeyId', required: true, masked: false },
        { name: 'accessKeySecret', required: true, masked: true },
    ],
    tencentcloud: [
        { name: 'secretId', required: true, masked: false },
        { name: 'secretKey', required: true, masked: true },
    ],
    route53: [
        { name: 'accessKeyId', required: true, masked: false },
        { name: 'secretAccessKey', required: true, masked: true },
        { name: 'region', required: true, masked: false },
        { name: 'hostedZoneId', required: false, masked: false },
    ],
}

export interface DnsCredential {
    id: string
    name: string
    provider: DnsProvider
    // Whether each value is stored; values themselves are never returned.
    fields: Partial<Record<SecretField, boolean>>
    certificates: number
    createdAt: string
    updatedAt: string
}

export const directoryKinds = [
    'letsencrypt',
    'letsencrypt-staging',
    'custom',
] as const
export type DirectoryKind = (typeof directoryKinds)[number]

export const certificateStatuses = [
    'pending',
    'valid',
    'failed',
    'expired',
] as const
export type CertificateStatus = (typeof certificateStatuses)[number]

export interface IssueProblem {
    code: string
    // Short ACME problem text from the CA, untranslated; absent for DNS errors.
    detail: string | null
    at: string
}

export interface Certificate {
    id: string
    domains: string[]
    directory: DirectoryKind
    directoryUrl: string
    email: string
    dnsCredentialId: string
    dnsCredentialName: string
    status: CertificateStatus
    issuing: boolean
    issuer: string | null
    notBefore: string | null
    notAfter: string | null
    renewAt: string | null
    lastError: IssueProblem | null
    createdAt: string
    updatedAt: string
}

export interface CertificateRequest {
    domains: string[]
    dnsCredentialId: string
    directory: DirectoryKind
    directoryUrl: string
    email: string
    eab: { keyId: string; hmacKey: string }
}

const idPattern = /^[A-Za-z0-9_-]{22}$/

function isCount(value: unknown): value is number {
    return (
        typeof value === 'number' && Number.isSafeInteger(value) && value >= 0
    )
}

function optionalDate(value: unknown): value is string | null {
    return value === null || isDate(value)
}

function isOneOf<T extends string>(
    values: readonly T[],
    value: unknown,
): value is T {
    return (
        typeof value === 'string' &&
        (values as readonly string[]).includes(value)
    )
}

function parseCredential(data: unknown): DnsCredential {
    if (
        !isRecord(data) ||
        typeof data.id !== 'string' ||
        !idPattern.test(data.id) ||
        typeof data.name !== 'string' ||
        !data.name ||
        !isOneOf(dnsProviders, data.provider) ||
        !isRecord(data.fields) ||
        !isCount(data.certificates) ||
        !isDate(data.createdAt) ||
        !isDate(data.updatedAt)
    ) {
        return invalidResponse()
    }
    const expected = providerFields[data.provider].map((field) => field.name)
    const received = data.fields
    if (
        Object.keys(received).length !== expected.length ||
        !expected.every((name) => typeof received[name] === 'boolean')
    ) {
        return invalidResponse()
    }
    return {
        id: data.id,
        name: data.name,
        provider: data.provider,
        fields: Object.fromEntries(
            expected.map((name) => [name, received[name] as boolean]),
        ),
        certificates: data.certificates,
        createdAt: data.createdAt,
        updatedAt: data.updatedAt,
    }
}

function parseProblem(value: unknown): IssueProblem | null {
    if (value === null) return null
    if (
        !isRecord(value) ||
        typeof value.code !== 'string' ||
        !/^[A-Z][A-Z_]{1,63}$/.test(value.code) ||
        !(value.detail === null || typeof value.detail === 'string') ||
        !isDate(value.at)
    ) {
        return invalidResponse()
    }
    return { code: value.code, detail: value.detail, at: value.at }
}

function parseCertificate(data: unknown): Certificate {
    if (
        !isRecord(data) ||
        typeof data.id !== 'string' ||
        !idPattern.test(data.id) ||
        !Array.isArray(data.domains) ||
        !data.domains.length ||
        data.domains.length > 100 ||
        !data.domains.every((name) => typeof name === 'string' && name) ||
        !isOneOf(directoryKinds, data.directory) ||
        typeof data.directoryUrl !== 'string' ||
        !data.directoryUrl.startsWith('https://') ||
        typeof data.email !== 'string' ||
        typeof data.dnsCredentialId !== 'string' ||
        !idPattern.test(data.dnsCredentialId) ||
        typeof data.dnsCredentialName !== 'string' ||
        !isOneOf(certificateStatuses, data.status) ||
        typeof data.issuing !== 'boolean' ||
        !(data.issuer === null || typeof data.issuer === 'string') ||
        !optionalDate(data.notBefore) ||
        !optionalDate(data.notAfter) ||
        !optionalDate(data.renewAt) ||
        !isDate(data.createdAt) ||
        !isDate(data.updatedAt)
    ) {
        return invalidResponse()
    }
    return {
        id: data.id,
        domains: data.domains as string[],
        directory: data.directory,
        directoryUrl: data.directoryUrl,
        email: data.email,
        dnsCredentialId: data.dnsCredentialId,
        dnsCredentialName: data.dnsCredentialName,
        status: data.status,
        issuing: data.issuing,
        issuer: data.issuer,
        notBefore: data.notBefore,
        notAfter: data.notAfter,
        renewAt: data.renewAt,
        lastError: parseProblem(data.lastError),
        createdAt: data.createdAt,
        updatedAt: data.updatedAt,
    }
}

function credentialResponse(data: unknown) {
    if (!isRecord(data)) return invalidResponse()
    return parseCredential(data.credential)
}

function certificateResponse(data: unknown) {
    if (!isRecord(data)) return invalidResponse()
    return parseCertificate(data.certificate)
}

function path(base: string, id: string, action: string) {
    return `${base}/${encodeURIComponent(id)}/${action}`
}

/** File name for a downloaded chain, derived from the first name. */
export function chainFileName(certificate: Certificate) {
    return `${(certificate.domains[0] ?? 'certificate').replace(/^\*\./, 'wildcard.')}.pem`
}

const pemChain =
    /^(?:-----BEGIN CERTIFICATE-----\r?\n[A-Za-z0-9+/=\r\n]+?-----END CERTIFICATE-----\s*)+$/

// The chain is PEM, not JSON, so it uses its own request with the same
// transport rules as apiRequest: same origin, no cache, no redirects, 10 s.
async function fetchChain(id: string) {
    let response: Response
    try {
        response = await fetch(path('/api/certificates', id, 'chain'), {
            credentials: 'same-origin',
            cache: 'no-store',
            redirect: 'error',
            signal: AbortSignal.timeout(10_000),
            headers: {
                Accept: 'application/pem-certificate-chain, application/json',
                'X-Requested-With': 'Veilpost',
            },
        })
    } catch {
        throw new AuthApiError('SERVICE_UNAVAILABLE')
    }
    if (!response.ok) {
        const data: unknown = await response.json().catch(() => null)
        const code =
            isRecord(data) && typeof data.code === 'string' ? data.code : ''
        if (response.status === 429) throw new AuthApiError('TOO_MANY_ATTEMPTS')
        if (response.status === 401 && !code)
            throw new AuthApiError('SESSION_EXPIRED')
        if (
            response.status >= 500 ||
            (response.status === 404 && !code.endsWith('_NOT_FOUND'))
        )
            throw new AuthApiError('SERVICE_UNAVAILABLE')
        throw new AuthApiError(code || 'REQUEST_FAILED')
    }
    const text = await response.text()
    if (
        !response.headers
            .get('content-type')
            ?.startsWith('application/pem-certificate-chain') ||
        !pemChain.test(text) ||
        text.includes('PRIVATE KEY')
    ) {
        return invalidResponse()
    }
    return text
}

export const certsApi = {
    async listCredentials(options: { background?: boolean } = {}) {
        const data = await apiRequest(
            '/api/dns-credentials',
            undefined,
            options,
        )
        if (!isRecord(data) || !Array.isArray(data.credentials))
            return invalidResponse()
        return data.credentials.map(parseCredential)
    },
    async createCredential(
        name: string,
        provider: DnsProvider,
        secrets: Partial<Record<SecretField, string>>,
        password: string,
    ) {
        const credential = credentialResponse(
            await apiRequest('/api/dns-credentials', {
                name,
                provider,
                secrets,
                password,
            }),
        )
        if (credential.provider !== provider) return invalidResponse()
        return credential
    },
    async updateCredential(
        id: string,
        name: string,
        secrets: Partial<Record<SecretField, string>>,
        password: string,
    ) {
        const credential = credentialResponse(
            await apiRequest(path('/api/dns-credentials', id, 'update'), {
                name,
                secrets,
                password,
            }),
        )
        if (credential.id !== id) return invalidResponse()
        return credential
    },
    async deleteCredential(id: string, password: string) {
        const data = await apiRequest(
            path('/api/dns-credentials', id, 'delete'),
            {
                password,
            },
        )
        if (data !== null) return invalidResponse()
    },
    async verifyCredential(id: string) {
        const data = await apiRequest(
            path('/api/dns-credentials', id, 'verify'),
            {},
        )
        if (!isRecord(data) || data.status !== 'ok') return invalidResponse()
    },
    async listCertificates(options: { background?: boolean } = {}) {
        const data = await apiRequest('/api/certificates', undefined, options)
        if (!isRecord(data) || !Array.isArray(data.certificates))
            return invalidResponse()
        return data.certificates.map(parseCertificate)
    },
    async request(input: CertificateRequest) {
        const certificate = certificateResponse(
            await apiRequest('/api/certificates', { ...input }),
        )
        if (certificate.dnsCredentialId !== input.dnsCredentialId)
            return invalidResponse()
        return certificate
    },
    async reissue(id: string) {
        const certificate = certificateResponse(
            await apiRequest(path('/api/certificates', id, 'reissue'), {}),
        )
        if (certificate.id !== id) return invalidResponse()
        return certificate
    },
    async remove(id: string) {
        const data = await apiRequest(
            path('/api/certificates', id, 'delete'),
            {},
        )
        if (data !== null) return invalidResponse()
    },
    /** The PEM chain (leaf first). Private keys are never downloadable. */
    async chain(id: string) {
        authActivity.pending++
        try {
            return await fetchChain(id)
        } finally {
            authActivity.pending--
        }
    },
}
