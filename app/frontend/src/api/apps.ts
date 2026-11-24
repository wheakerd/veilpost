import { apiRequest, invalidResponse, isDate, isRecord } from './auth'

// Contract: docs/apps-api.md. Sensitive settings are write-only: the server
// reports only whether each one is set, and values typed into the form stay
// in component memory until they are sent.

export type SettingType =
    | 'port'
    | 'integer'
    | 'string'
    | 'enum'
    | 'boolean'
    | 'hostname'
    | 'hostport'
    | 'url'

export type SettingValue = string | number | boolean | null

export interface AppSetting {
    key: string
    type: SettingType
    // Translation keys under apps.catalog.<id> in the language packs.
    label: string
    hint: string | null
    required: boolean
    requiredWhen: { setting: string; equals: string } | null
    default: SettingValue
    min: number | null
    max: number | null
    minLength: number | null
    maxLength: number | null
    pattern: string | null
    options: { value: string; label: string }[]
    sensitive: boolean
}

export interface AppProtocol {
    id: string
    name: string
    transport: 'tcp' | 'udp'
}

export interface AppPort {
    setting: string
    protocol: string
    transport: 'tcp' | 'udp'
    // Null: chosen per node at deployment.
    port: number | null
}

export interface AppImage {
    // Digest-pinned pull reference; empty for images built from this repository.
    reference: string
    repository: string | null
    tag: string | null
    digest: string | null
    platforms: string[]
    build: string | null
    source: string
}

export interface Installation {
    version: string
    installedAt: string
    updatedAt: string
    configured: boolean
}

export interface StoreApp {
    id: string
    version: string
    name: string
    description: string
    protocols: AppProtocol[]
    ports: AppPort[]
    settings: AppSetting[]
    image: AppImage
    installed: Installation | null
}

export interface AppSettings {
    values: Record<string, SettingValue>
    secrets: Record<string, boolean>
    configured: boolean
    updatedAt: string
}

export interface SettingsUpdate {
    values: Record<string, SettingValue>
    // Omitted keys keep the stored secret; null clears it.
    secrets: Record<string, string | null>
}

export const appIdPattern = /^[a-z][a-z0-9-]{1,31}$/
const settingTypes: readonly string[] = [
    'port',
    'integer',
    'string',
    'enum',
    'boolean',
    'hostname',
    'hostport',
    'url',
]

function isKey(value: unknown, id: string): value is string {
    return (
        typeof value === 'string' &&
        value.startsWith(`apps.catalog.${id}.`) &&
        /^[\w.]+$/.test(value)
    )
}

function isString(value: unknown): value is string {
    return typeof value === 'string' && value.length > 0
}

function isInteger(value: unknown): value is number {
    return typeof value === 'number' && Number.isSafeInteger(value)
}

function optional<T>(value: unknown, check: (value: unknown) => value is T) {
    if (value === undefined) return null
    return check(value) ? value : invalidResponse()
}

function isTransport(value: unknown): value is 'tcp' | 'udp' {
    return value === 'tcp' || value === 'udp'
}

function isValue(setting: AppSetting, value: unknown): value is SettingValue {
    if (value === null) return true
    switch (setting.type) {
        case 'port':
            return isInteger(value) && value >= 1 && value <= 65535
        case 'integer':
            return (
                isInteger(value) &&
                (setting.min == null || value >= setting.min) &&
                (setting.max == null || value <= setting.max)
            )
        case 'boolean':
            return typeof value === 'boolean'
        case 'enum':
            return setting.options.some((option) => option.value === value)
        default:
            return isString(value)
    }
}

function parseSetting(data: unknown, id: string): AppSetting {
    if (
        !isRecord(data) ||
        typeof data.key !== 'string' ||
        !/^[a-z][A-Za-z0-9]{0,31}$/.test(data.key) ||
        typeof data.type !== 'string' ||
        !settingTypes.includes(data.type) ||
        !isKey(data.label, id)
    )
        return invalidResponse()
    const condition = data.requiredWhen
    if (
        condition !== undefined &&
        !(
            isRecord(condition) &&
            isString(condition.setting) &&
            isString(condition.equals)
        )
    )
        return invalidResponse()
    const options = data.options ?? []
    if (
        !Array.isArray(options) ||
        !options.every(
            (option) =>
                isRecord(option) &&
                isString(option.value) &&
                isKey(option.label, id),
        )
    )
        return invalidResponse()
    const setting: AppSetting = {
        key: data.key,
        type: data.type as SettingType,
        label: data.label,
        hint: optional(data.hint, (value) => isKey(value, id)),
        required: optional(data.required, (value) => value === true) ?? false,
        requiredWhen: condition
            ? {
                  setting: condition.setting as string,
                  equals: condition.equals as string,
              }
            : null,
        default: null,
        min: optional(data.min, isInteger),
        max: optional(data.max, isInteger),
        minLength: optional(data.minLength, isInteger),
        maxLength: optional(data.maxLength, isInteger),
        pattern: optional(data.pattern, isString),
        options: (options as { value: string; label: string }[]).map(
            ({ value, label }) => ({ value, label }),
        ),
        sensitive: optional(data.sensitive, (value) => value === true) ?? false,
    }
    if (
        (setting.type === 'integer' &&
            (setting.min === null || setting.max === null)) ||
        (setting.type === 'enum') !== setting.options.length > 0 ||
        (setting.sensitive && setting.type !== 'string')
    )
        return invalidResponse()
    if (data.default !== undefined) {
        if (data.default === null || !isValue(setting, data.default))
            return invalidResponse()
        setting.default = data.default
    }
    return setting
}

function parseImage(data: unknown): AppImage {
    if (
        !isRecord(data) ||
        typeof data.reference !== 'string' ||
        typeof data.source !== 'string' ||
        !data.source.startsWith('https://github.com/')
    )
        return invalidResponse()
    const image: AppImage = {
        reference: data.reference,
        repository: optional(data.repository, isString),
        tag: optional(data.tag, isString),
        digest: optional(
            data.digest,
            (value): value is string =>
                isString(value) && /^sha256:[0-9a-f]{64}$/.test(value),
        ),
        platforms:
            optional(
                data.platforms,
                (value): value is string[] =>
                    Array.isArray(value) && value.every(isString),
            ) ?? [],
        build: optional(data.build, isString),
        source: data.source,
    }
    // Exactly one of a digest-pinned reference and a local build directory.
    const pinned =
        image.repository !== null &&
        image.tag !== null &&
        image.digest !== null &&
        image.reference ===
            `${image.repository}:${image.tag}@${image.digest}` &&
        image.build === null
    const local =
        image.build !== null && image.reference === '' && image.digest === null
    if (pinned === local) return invalidResponse()
    return image
}

function parseInstallation(data: unknown): Installation | null {
    if (data === null) return null
    if (
        !isRecord(data) ||
        !isString(data.version) ||
        !isDate(data.installedAt) ||
        !isDate(data.updatedAt) ||
        typeof data.configured !== 'boolean'
    )
        return invalidResponse()
    return {
        version: data.version,
        installedAt: data.installedAt,
        updatedAt: data.updatedAt,
        configured: data.configured,
    }
}

function parseApp(data: unknown): StoreApp {
    if (
        !isRecord(data) ||
        typeof data.id !== 'string' ||
        !appIdPattern.test(data.id) ||
        !isString(data.version) ||
        !Array.isArray(data.protocols) ||
        !data.protocols.length ||
        !Array.isArray(data.ports) ||
        !Array.isArray(data.settings)
    )
        return invalidResponse()
    const id = data.id
    if (!isKey(data.name, id) || !isKey(data.description, id))
        return invalidResponse()
    const protocols = data.protocols.map((protocol): AppProtocol => {
        if (
            !isRecord(protocol) ||
            !isString(protocol.id) ||
            !isString(protocol.name) ||
            !isTransport(protocol.transport)
        )
            return invalidResponse()
        return {
            id: protocol.id,
            name: protocol.name,
            transport: protocol.transport,
        }
    })
    const settings = data.settings.map((setting) => parseSetting(setting, id))
    const keys = new Set(settings.map((setting) => setting.key))
    if (keys.size !== settings.length) return invalidResponse()
    const ports = data.ports.map((port): AppPort => {
        if (
            !isRecord(port) ||
            typeof port.setting !== 'string' ||
            !keys.has(port.setting) ||
            !protocols.some((protocol) => protocol.id === port.protocol) ||
            !isTransport(port.transport) ||
            !(
                port.port === null ||
                (isInteger(port.port) && port.port >= 1 && port.port <= 65535)
            )
        )
            return invalidResponse()
        return {
            setting: port.setting,
            protocol: port.protocol as string,
            transport: port.transport,
            port: port.port,
        }
    })
    return {
        id,
        version: data.version,
        name: data.name,
        description: data.description,
        protocols,
        ports,
        settings,
        image: parseImage(data.image),
        installed: parseInstallation(data.installed),
    }
}

/** Validates settings against the app's manifest: no extra or missing keys. */
export function parseSettings(app: StoreApp, data: unknown): AppSettings {
    if (
        !isRecord(data) ||
        !isRecord(data.values) ||
        !isRecord(data.secrets) ||
        typeof data.configured !== 'boolean' ||
        !isDate(data.updatedAt)
    )
        return invalidResponse()
    const values: Record<string, SettingValue> = {}
    const secrets: Record<string, boolean> = {}
    for (const setting of app.settings) {
        if (setting.sensitive) {
            const set = data.secrets[setting.key]
            if (typeof set !== 'boolean') return invalidResponse()
            secrets[setting.key] = set
        } else {
            const value = data.values[setting.key]
            if (!isValue(setting, value)) return invalidResponse()
            values[setting.key] = value
        }
    }
    if (
        Object.keys(data.values).length !== Object.keys(values).length ||
        Object.keys(data.secrets).length !== Object.keys(secrets).length
    )
        return invalidResponse()
    return {
        values,
        secrets,
        configured: data.configured,
        updatedAt: data.updatedAt,
    }
}

function path(id: string, action: string) {
    return `/api/apps/${encodeURIComponent(id)}/${action}`
}

export const appsApi = {
    async list(options: { background?: boolean } = {}) {
        const data = await apiRequest('/api/apps', undefined, options)
        if (!isRecord(data) || !Array.isArray(data.apps))
            return invalidResponse()
        const apps = data.apps.map(parseApp)
        if (new Set(apps.map((app) => app.id)).size !== apps.length)
            return invalidResponse()
        return apps
    },
    async install(id: string) {
        const data = await apiRequest(path(id, 'install'), {})
        if (!isRecord(data)) return invalidResponse()
        const app = parseApp(data.app)
        if (app.id !== id || !app.installed) return invalidResponse()
        return app
    },
    async uninstall(id: string, password: string) {
        const data = await apiRequest(path(id, 'uninstall'), { password })
        if (data !== null) return invalidResponse()
    },
    async settings(app: StoreApp) {
        return parseSettings(app, await apiRequest(path(app.id, 'settings')))
    },
    async updateSettings(app: StoreApp, update: SettingsUpdate) {
        return parseSettings(
            app,
            await apiRequest(path(app.id, 'settings'), { ...update }),
        )
    },
}
