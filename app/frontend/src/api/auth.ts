import { i18n, isErrorCode } from '@/i18n'
import { authActivity } from './activity'

export interface AuthUser {
    id: string
    username: string
    twoFactorEnabled: boolean
}

export type AuthSession =
    | { status: 'anonymous'; setupRequired?: boolean }
    | { status: 'mfa_required'; expiresAt: string }
    | { status: 'authenticated'; user: AuthUser }

export interface SecurityStatus {
    recoveryCodesRemaining: number
    otherSessions: number
}

export interface TwoFactorSetup {
    setupId: string
    secret: string
    expiresAt: string
    qrCodeDataUrl?: string
}

export class AuthApiError extends Error {
    // field names the rejected setting of INVALID_SETTING and SETTING_REQUIRED
    // (docs/apps-api.md); it is a manifest key, never a value.
    constructor(
        public code: string,
        public field?: string,
    ) {
        // The message is the code; user-facing text comes from authErrorMessage.
        super(code)
    }
}

// 404 codes of console resources; any other 404 means the API is missing.
const resourceNotFound = new Set([
    'NODE_NOT_FOUND',
    'APP_NOT_FOUND',
    'APP_NOT_INSTALLED',
    'CREDENTIAL_NOT_FOUND',
    'CERTIFICATE_NOT_FOUND',
])

/**
 * The localized text for an API error. The backend returns only codes; codes
 * without text in the language packs are shown as REQUEST_FAILED.
 */
export function authErrorMessage(error: unknown) {
    const code =
        error instanceof AuthApiError && isErrorCode(error.code)
            ? error.code
            : 'REQUEST_FAILED'
    return i18n.global.t(`errors.${code}`)
}

export function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === 'object' && value !== null
}

export function invalidResponse(): never {
    throw new AuthApiError('SERVICE_UNAVAILABLE')
}

export function isDate(value: unknown): value is string {
    return typeof value === 'string' && Number.isFinite(Date.parse(value))
}

function parseSession(data: unknown): AuthSession {
    if (!isRecord(data)) return invalidResponse()
    if (data.status === 'anonymous') {
        if (
            data.setupRequired !== undefined &&
            typeof data.setupRequired !== 'boolean'
        )
            return invalidResponse()
        return {
            status: 'anonymous',
            setupRequired: data.setupRequired,
        }
    }
    if (data.status === 'mfa_required' && isDate(data.expiresAt)) {
        return { status: 'mfa_required', expiresAt: data.expiresAt }
    }
    const user = data.user
    if (
        data.status === 'authenticated' &&
        isRecord(user) &&
        typeof user.id === 'string' &&
        typeof user.username === 'string' &&
        typeof user.twoFactorEnabled === 'boolean'
    ) {
        return {
            status: 'authenticated',
            user: {
                id: user.id,
                username: user.username,
                twoFactorEnabled: user.twoFactorEnabled,
            },
        }
    }
    return invalidResponse()
}

// Session/challenge cookies are owned by the server. Never store credentials here.
async function performRequest(url: string, body?: Record<string, unknown>) {
    let response: Response
    try {
        response = await fetch(url, {
            method: body === undefined ? 'GET' : 'POST',
            credentials: 'same-origin',
            cache: 'no-store',
            redirect: 'error',
            signal: AbortSignal.timeout(10_000),
            headers: {
                Accept: 'application/json',
                'X-Requested-With': 'Veilpost',
                ...(body === undefined
                    ? {}
                    : { 'Content-Type': 'application/json' }),
            },
            ...(body === undefined ? {} : { body: JSON.stringify(body) }),
        })
    } catch {
        throw new AuthApiError('SERVICE_UNAVAILABLE')
    }

    const data: unknown = response.headers
        .get('content-type')
        ?.includes('application/json')
        ? await response.json().catch(() => null)
        : null

    if (!response.ok) {
        const code =
            isRecord(data) && typeof data.code === 'string' ? data.code : ''
        if (response.status === 429) throw new AuthApiError('TOO_MANY_ATTEMPTS')
        if (response.status === 401 && !code) {
            throw new AuthApiError('SESSION_EXPIRED')
        }
        // A 404 without a known resource code means the API itself is missing.
        if (
            response.status >= 500 ||
            (response.status === 404 && !resourceNotFound.has(code))
        ) {
            throw new AuthApiError('SERVICE_UNAVAILABLE')
        }
        const field =
            isRecord(data) && typeof data.field === 'string'
                ? data.field
                : undefined
        throw new AuthApiError(code || 'REQUEST_FAILED', field)
    }
    if (response.status === 204) return null
    if (data === null) return invalidResponse()
    return data
}

/**
 * Same-origin console API request with the shared transport rules: JSON,
 * the CSRF header, no redirects, a 10 s timeout and no automatic retries.
 * Background requests (periodic refreshes) do not show the progress bar.
 */
export async function apiRequest(
    url: string,
    body?: Record<string, unknown>,
    options: { background?: boolean } = {},
) {
    if (options.background) return performRequest(url, body)
    authActivity.pending++
    try {
        return await performRequest(url, body)
    } finally {
        authActivity.pending--
    }
}

function request(path: string, body?: Record<string, unknown>) {
    return apiRequest(`/api/auth${path}`, body)
}

function isCount(value: unknown): value is number {
    return (
        typeof value === 'number' && Number.isSafeInteger(value) && value >= 0
    )
}

function parseRecoveryCodes(data: unknown) {
    if (
        !isRecord(data) ||
        !Array.isArray(data.recoveryCodes) ||
        !data.recoveryCodes.length ||
        !data.recoveryCodes.every(
            (code) => typeof code === 'string' && code.length,
        )
    ) {
        return invalidResponse()
    }
    return data.recoveryCodes as string[]
}

function authenticated(data: unknown, twoFactorEnabled?: boolean) {
    const session = parseSession(data)
    if (
        session.status !== 'authenticated' ||
        (twoFactorEnabled !== undefined &&
            session.user.twoFactorEnabled !== twoFactorEnabled)
    ) {
        return invalidResponse()
    }
    return session
}

export const authApi = {
    async session() {
        try {
            return parseSession(await request('/session'))
        } catch (error) {
            if (
                error instanceof AuthApiError &&
                error.code === 'SESSION_EXPIRED'
            ) {
                return { status: 'anonymous' } as const
            }
            throw error
        }
    },
    async login(username: string, password: string) {
        const session = parseSession(
            await request('/login', { username, password }),
        )
        if (session.status === 'anonymous') return invalidResponse()
        return session
    },
    async verify(code: string, method: 'totp' | 'recovery') {
        return authenticated(await request('/verify', { code, method }))
    },
    async logout() {
        await request('/logout', {})
    },
    async beginSetup(password: string): Promise<TwoFactorSetup> {
        const data = await request('/two-factor/setup', { password })
        if (
            !isRecord(data) ||
            typeof data.setupId !== 'string' ||
            typeof data.secret !== 'string' ||
            !/^[A-Z2-7]{16,128}$/.test(data.secret) ||
            !isDate(data.expiresAt)
        ) {
            return invalidResponse()
        }
        return {
            setupId: data.setupId,
            secret: data.secret,
            expiresAt: data.expiresAt,
            // Only an inline PNG is allowed: no secret-bearing third-party QR URLs.
            qrCodeDataUrl:
                typeof data.qrCodeDataUrl === 'string' &&
                /^data:image\/png;base64,[A-Za-z0-9+/=]+$/.test(
                    data.qrCodeDataUrl,
                )
                    ? data.qrCodeDataUrl
                    : undefined,
        }
    },
    async confirmSetup(setupId: string, code: string) {
        const data = await request('/two-factor/enable', { setupId, code })
        const session = authenticated(data, true)
        return { session, recoveryCodes: parseRecoveryCodes(data) }
    },
    async regenerateRecoveryCodes(password: string, code: string) {
        return parseRecoveryCodes(
            await request('/two-factor/recovery-codes', { password, code }),
        )
    },
    async securityStatus(): Promise<SecurityStatus> {
        const data = await request('/security')
        if (
            !isRecord(data) ||
            !isCount(data.recoveryCodesRemaining) ||
            !isCount(data.otherSessions)
        ) {
            return invalidResponse()
        }
        return {
            recoveryCodesRemaining: data.recoveryCodesRemaining,
            otherSessions: data.otherSessions,
        }
    },
    async changePassword(password: string, newPassword: string) {
        return authenticated(
            await request('/password', { password, newPassword }),
        )
    },
    async revokeOtherSessions() {
        await request('/sessions/revoke-others', {})
    },
    async disable(password: string, code: string) {
        return authenticated(
            await request('/two-factor/disable', { password, code }),
            false,
        )
    },
}
