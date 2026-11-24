import type { OverviewMessages } from '../zh-CN/overview'

export default {
    description: 'Live status of nodes, software and certificates',
    viewNodes: 'View VPN nodes',
    summary: 'Summary',
    online: '{online} / {total} online',
    ofTotal: '{count} / {total}',
    stats: {
        nodes: 'Nodes online',
        nodesDetail: '{pending} awaiting enrollment',
        apps: 'Installed software',
        appsDetail: 'VPN software from the built-in catalog',
        certificates: 'Valid certificates',
        nextExpiry: 'Next expiry {date}',
        noExpiry: 'No certificate expiring soon',
        twoFactor: 'Two-step verification',
        enabled: 'On',
        disabled: 'Off',
        manage: 'Manage',
    },
    nodes: {
        title: 'Node status',
        all: 'All nodes',
        name: 'Name',
        status: 'Status',
        lastSeen: 'Last contact',
        states: {
            online: 'Online',
            offline: 'Offline',
            pending: 'Awaiting enrollment',
        },
        empty: 'No nodes yet. Create a node and run the enrollment command on its server to see its status here.',
        unavailable: 'Node status is unavailable right now',
    },
    expiry: {
        title: 'Certificates expiring next',
        all: 'All certificates',
        empty: 'No valid certificates yet',
        unavailable: 'Certificates are unavailable right now',
    },
} satisfies OverviewMessages
