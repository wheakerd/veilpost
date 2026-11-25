import { apiRequest, invalidResponse, isDate, isRecord } from './auth'

// Contract: docs/nodes-api.md. Enrollment tokens are returned only by create
// and reissue and must stay in component memory: never in storage, URLs or logs.

export type NodeStatus = 'pending' | 'active' | 'revoked'

const dockerStatuses = [
    'queued',
    'running',
    'succeeded',
    'already_installed',
    'failed',
    'expired',
    'unknown',
] as const
const dockerErrors = [
    'source_unavailable',
    'install_failed',
    'timed_out',
] as const

export interface DockerInstallation {
    id: string
    source: string
    status: (typeof dockerStatuses)[number]
    updatedAt: string
    error?: (typeof dockerErrors)[number]
}

interface NodeDocker {
    sources: string[]
    job?: DockerInstallation
}

export interface VpnNode {
    id: string
    name: string
    status: NodeStatus
    online: boolean
    agentVersion: string | null
    createdAt: string
    enrolledAt: string | null
    revokedAt: string | null
    lastSeenAt: string | null
    enrollmentExpiresAt: string | null
    docker?: NodeDocker
}

export interface NodeEnrollment {
    token: string
    expiresAt: string
}

const statuses: readonly string[] = ['pending', 'active', 'revoked']
const dockerSourcePattern = /^[a-z0-9][a-z0-9-]{0,31}$/
const nodeIdPattern = /^[A-Za-z0-9_-]{22}$/

function isDockerSource(value: unknown): value is string {
    return typeof value === 'string' && dockerSourcePattern.test(value)
}

function parseDocker(value: unknown): NodeDocker | undefined {
    if (value === undefined) return
    if (
        !isRecord(value) ||
        !Array.isArray(value.sources) ||
        value.sources.length > 16 ||
        !value.sources.every(isDockerSource)
    )
        return invalidResponse()
    if (value.job === undefined) return { sources: value.sources }
    const job = value.job
    if (
        !isRecord(job) ||
        typeof job.id !== 'string' ||
        !nodeIdPattern.test(job.id) ||
        !isDockerSource(job.source) ||
        typeof job.status !== 'string' ||
        !(dockerStatuses as readonly string[]).includes(job.status) ||
        !isDate(job.updatedAt) ||
        (job.error !== undefined &&
            (typeof job.error !== 'string' ||
                !(dockerErrors as readonly string[]).includes(job.error)))
    )
        return invalidResponse()
    return {
        sources: value.sources,
        job: {
            id: job.id,
            source: job.source,
            status: job.status as DockerInstallation['status'],
            updatedAt: job.updatedAt,
            error: job.error as DockerInstallation['error'],
        },
    }
}

function optionalDate(value: unknown): value is string | null {
    return value === null || isDate(value)
}

function parseNode(data: unknown): VpnNode {
    if (
        !isRecord(data) ||
        typeof data.id !== 'string' ||
        !nodeIdPattern.test(data.id) ||
        typeof data.name !== 'string' ||
        !data.name ||
        typeof data.status !== 'string' ||
        !statuses.includes(data.status) ||
        typeof data.online !== 'boolean' ||
        !(
            data.agentVersion === null || typeof data.agentVersion === 'string'
        ) ||
        !isDate(data.createdAt) ||
        !optionalDate(data.enrolledAt) ||
        !optionalDate(data.revokedAt) ||
        !optionalDate(data.lastSeenAt) ||
        !optionalDate(data.enrollmentExpiresAt)
    ) {
        return invalidResponse()
    }
    return {
        id: data.id,
        name: data.name,
        status: data.status as NodeStatus,
        // Only an enrolled, unrevoked node can be online.
        online: data.status === 'active' && data.online,
        agentVersion: data.agentVersion,
        createdAt: data.createdAt,
        enrolledAt: data.enrolledAt,
        revokedAt: data.revokedAt,
        lastSeenAt: data.lastSeenAt,
        enrollmentExpiresAt: data.enrollmentExpiresAt,
        docker: parseDocker(data.docker),
    }
}

function parseNodeResponse(data: unknown) {
    if (!isRecord(data)) return invalidResponse()
    return parseNode(data.node)
}

function parseEnrollment(data: unknown) {
    if (
        !isRecord(data) ||
        !isRecord(data.enrollment) ||
        typeof data.enrollment.token !== 'string' ||
        !/^[A-Za-z0-9_-]{43}$/.test(data.enrollment.token) ||
        !isDate(data.enrollment.expiresAt)
    ) {
        return invalidResponse()
    }
    const node = parseNodeResponse(data)
    if (node.status !== 'pending') return invalidResponse()
    const enrollment: NodeEnrollment = {
        token: data.enrollment.token,
        expiresAt: data.enrollment.expiresAt,
    }
    return { node, enrollment }
}

export const nodesApi = {
    async list(options: { background?: boolean } = {}) {
        const data = await apiRequest('/api/nodes', undefined, options)
        if (!isRecord(data) || !Array.isArray(data.nodes))
            return invalidResponse()
        return data.nodes.map(parseNode)
    },
    async create(name: string, password: string) {
        return parseEnrollment(
            await apiRequest('/api/nodes', { name, password }),
        )
    },
    async reissue(id: string, password: string) {
        const result = parseEnrollment(
            await apiRequest(
                `/api/nodes/${encodeURIComponent(id)}/enrollment`,
                {
                    password,
                },
            ),
        )
        if (result.node.id !== id) return invalidResponse()
        return result
    },
    async remove(id: string, password: string) {
        const data = await apiRequest(
            `/api/nodes/${encodeURIComponent(id)}/delete`,
            { password },
        )
        if (data !== null) return invalidResponse()
    },
    async revoke(id: string, password: string) {
        const node = parseNodeResponse(
            await apiRequest(`/api/nodes/${encodeURIComponent(id)}/revoke`, {
                password,
            }),
        )
        if (node.id !== id || node.status !== 'revoked')
            return invalidResponse()
        return node
    },
    async installDocker(id: string, source: string, password: string) {
        const node = parseNodeResponse(
            await apiRequest(
                `/api/nodes/${encodeURIComponent(id)}/docker/install`,
                {
                    source,
                    password,
                },
            ),
        )
        if (node.id !== id || !node.docker?.job) return invalidResponse()
        return node
    },
}
