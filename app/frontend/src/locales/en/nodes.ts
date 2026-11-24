import type { NodesMessages } from '../zh-CN/nodes'

export default {
    description: 'Bring nodes of every protocol into one workspace',
    create: 'Create node',
    listTitle: 'Node list',
    listLabel: 'Nodes',
    columns: {
        name: 'Name',
        status: 'Status',
        id: 'Node ID',
        actions: 'Actions',
    },
    copyId: 'Copy the node ID of “{name}”',
    onlineCount: '{count} online',
    form: {
        description:
            'Creating a node generates a one-time enrollment token. It’s valid for 15 minutes and shown only once. Enter your current password to confirm.',
        name: 'Node name',
        nameHint:
            '1–64 characters. Only used to tell nodes apart in the console.',
        submit: 'Create and generate token',
        missing: 'Enter a node name and your current password.',
        missingPassword: 'Enter your current password.',
    },
    enrollment: {
        title: 'Enroll node “{name}”',
        instructions:
            'Run the command below on the node server, paste the enrollment token when prompted, and press Enter. The token is valid until {time}, works only once, and can’t be viewed again after you close this panel.',
        command: 'Enrollment command',
        token: 'Enrollment token',
        copyCommand: 'Copy command',
        copyToken: 'Copy token',
        note: 'The token is passed on standard input. Never put it in command-line arguments or scripts. The default state directory is {stateDir}, which usually requires root. After enrolling, run {runCommand} to stay connected.',
        finish: 'Done, hide token',
        hidden: 'Enrollment token hidden. Node “{name}” will show as online once it finishes enrolling.',
        expired:
            'The enrollment token has expired. You can reissue one from the node list.',
    },
    reissue: {
        open: 'Reissue token',
        description:
            'Reissuing immediately invalidates this node’s previous enrollment token. The new token is valid for 15 minutes and shown only once. Enter your current password to confirm.',
        submit: 'Issue new token',
    },
    revoke: {
        open: 'Revoke',
        description:
            'Revoking immediately invalidates this node’s signatures, and its agent stops connecting. To reconnect, create a new node. Enter your current password to confirm.',
        submit: 'Revoke node',
        done: 'Node “{name}” revoked. Its agent can no longer connect to the console.',
    },
    delete: {
        open: 'Delete',
        description:
            'Deleting permanently removes this node’s record from the list. Enter your current password to confirm.',
        submit: 'Delete node',
        done: 'Node “{name}” deleted',
    },
    state: {
        revoked: 'Revoked',
        pending: 'Awaiting enrollment',
        expired: 'Token expired',
        online: 'Online',
        offline: 'Offline',
    },
    detail: {
        revokedAt: 'Revoked {time}',
        tokenValidUntil: 'Enrollment token valid until {time}',
        tokenExpired: 'Enrollment token expired. You can reissue it.',
        lastSeen: 'Last contact: {time}',
        neverSeen: 'Enrolled. No signed request from the agent yet.',
    },
    meta: {
        agentVersion: 'Agent version',
        unknown: 'Unknown',
        createdAt: 'Created',
    },
    unavailable: {
        stale: 'Can’t refresh node status right now. Showing the last loaded results.',
        empty: 'Can’t load the node list right now. Refresh the page later.',
    },
    empty: {
        title: 'Your VPN nodes will appear here',
        first: 'No VPN nodes are connected yet',
        second: 'Create a node and run the enrollment command on the server to see its real online status here',
    },
    protocols: 'Entry protocols come from the VPN software in the store',
    storeLink: 'Go to the store',
    note: 'Only node enrollment and online status are available for now. VPN configuration delivery, entry protocols, and traffic monitoring aren’t available yet.',
} satisfies NodesMessages
