import { apiRequest, invalidResponse, isDate, isRecord } from './auth'

// Contract: docs/nodes-api.md. Enrollment tokens are returned only by create
// and reissue and must stay in component memory: never in storage, URLs or logs.

export type NodeStatus = 'pending' | 'active' | 'revoked'

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
}

export interface NodeEnrollment {
    token: string
    expiresAt: string
}

const statuses: readonly string[] = ['pending', 'active', 'revoked']

function optionalDate(value: unknown): value is string | null {
    return value === null || isDate(value)
}

function parseNode(data: unknown): VpnNode {
    if (
        !isRecord(data) ||
        typeof data.id !== 'string' ||
        !/^[A-Za-z0-9_-]{22}$/.test(data.id) ||
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
}
