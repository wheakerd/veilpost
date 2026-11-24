import type { AppsMessages } from '../zh-CN/apps'

export default {
    store: {
        description:
            'Enable VPN software to add its settings and management page to the sidebar',
        listTitle: 'Catalog',
        listLabel: 'Software',
        installedCount: '{count} installed',
        builtIn:
            'The catalog ships with each console release and pins images by digest. Remote sources and custom uploads aren’t supported.',
        version: 'Version {version}',
        protocols: 'Protocols',
        ports: 'Ports',
        image: 'Image',
        randomPort: 'Random per node at deployment',
        localBuild: 'Built from {path} in this repository; not published yet',
        state: {
            notInstalled: 'Not installed',
            installed: 'Installed',
            needsSettings: 'Needs settings',
        },
        install: 'Install',
        installedNotice: '{name} is installed. Its page is now in the sidebar.',
        manage: 'Settings and management',
        unavailable: {
            stale: 'Can’t refresh the catalog right now. Showing the last result.',
            empty: 'Can’t load the catalog right now. Refresh the page later.',
        },
    },
    uninstall: {
        open: 'Uninstall',
        description:
            'Uninstalling deletes all settings of this software, including saved sensitive values. This can’t be undone. Enter your current password to confirm.',
        submit: 'Uninstall',
        done: '{name} is uninstalled and its settings are deleted',
        missingPassword: 'Enter your current password.',
    },
    app: {
        notInstalled: {
            title: '{name} isn’t installed',
            description:
                'Install it from the store to see its settings and management page here',
        },
        notFound: {
            title: 'This software isn’t in the catalog',
            description:
                'Check the address, or see the available software in the store',
        },
        toStore: 'Go to the store',
        unavailable:
            'Can’t load this software right now. Refresh the page later.',
        settings: {
            title: 'Settings',
            description:
                'The software manifest defines these settings and the server validates them. Sensitive values are write-only: after saving, only whether they are set is shown.',
            complete: 'Complete',
            incomplete: 'Needs settings: some required fields are empty',
            required: 'Required',
            secretSet: 'Set',
            secretUnset: 'Not set',
            secretKeep: 'Leave empty to keep the saved value',
            secretClear: 'Clear saved value',
            secretClearing: 'The saved value will be cleared when you save',
            secretUndo: 'Keep saved value',
            enabled: 'On',
            save: 'Save settings',
            saved: 'Settings saved',
            invalidField:
                '“{field}” isn’t valid. Follow the hint and try again.',
            requiredField: 'Fill in “{field}”.',
            updatedAt: 'Last changed {time}',
        },
        management: {
            title: 'Management',
            version: 'Version',
            installedAt: 'Installed',
            image: 'Image',
            platforms: 'Platforms',
            source: 'Upstream source',
            deployTitle: 'Deploy to nodes',
            deployUnavailable:
                'Not available yet: the node agent must first support configuration delivery (node phase 2). Until then, no node runs this software.',
            deployAction: 'Deploy to nodes',
        },
    },
    catalog: {
        xray: {
            name: 'Xray-core',
            description:
                'Two TCP entries, VLESS + REALITY + Vision and VLESS + XHTTP + REALITY, that borrow the TLS handshake of a real website',
            settings: {
                visionPort: {
                    label: 'Vision entry port',
                    hint: 'TCP port. Defaults to 443.',
                },
                visionTarget: {
                    label: 'Vision borrowed site (target)',
                    hint: 'A domain or “domain:port”; the port defaults to 443. The site must support TLS 1.3 and HTTP/2 without redirects. Required, with no default.',
                },
                visionServerName: {
                    label: 'Vision server name (serverName)',
                    hint: 'The SNI clients send. Leave empty to use the borrowed site’s domain.',
                },
                xhttpPort: {
                    label: 'XHTTP entry port',
                    hint: 'TCP port. Leave empty to pick a random high port per node at deployment.',
                },
                xhttpTarget: {
                    label: 'XHTTP borrowed site (target)',
                    hint: 'Can differ from the Vision entry; same format and requirements. Required, with no default.',
                },
                xhttpServerName: {
                    label: 'XHTTP server name (serverName)',
                    hint: 'Leave empty to use the borrowed site’s domain',
                },
                xhttpPath: {
                    label: 'XHTTP path',
                    hint: 'Starts with /, 2–64 characters of letters, digits and . _ ~ -. Leave empty to generate one per node at deployment.',
                },
                xhttpMode: {
                    label: 'XHTTP mode',
                    hint: 'Set explicitly; auto isn’t used',
                    options: {
                        streamOne: 'stream-one',
                        streamUp: 'stream-up',
                        packetUp: 'packet-up',
                    },
                },
            },
        },
        hysteria2: {
            name: 'Hysteria 2',
            description:
                'The official server with a UDP (QUIC) entry. Masquerade answers HTTP/3 probes; Salamander obfuscation hides QUIC traits.',
            settings: {
                port: {
                    label: 'UDP port',
                    hint: 'Defaults to 443; it doesn’t conflict with a TCP 443 entry',
                },
                obfuscation: {
                    label: 'Disguise',
                    hint: 'Masquerade resists active probing. Salamander looks like random UDP but can’t answer probes.',
                    options: {
                        masquerade: 'Masquerade as an HTTP/3 site',
                        salamander: 'Salamander obfuscation',
                    },
                },
                masqueradeUrl: {
                    label: 'Masquerade site URL',
                    hint: 'This https site answers unauthenticated requests. Required for masquerade.',
                },
                salamanderPassword: {
                    label: 'Salamander password',
                    hint: '16–128 characters. Required for Salamander. It isn’t shown again after saving.',
                },
            },
        },
        amneziawg: {
            name: 'AmneziaWG',
            description:
                'A UDP entry with AmneziaWG 2.0 obfuscation parameters generated per node. The image is built from this repository.',
            settings: {
                port: {
                    label: 'UDP port',
                    hint: 'UDP port. Leave empty to pick a random high port per node at deployment; UDP 443 is left to Hysteria 2.',
                },
                mtu: {
                    label: 'MTU',
                    hint: '1280–1420. Defaults to 1280.',
                },
                requireIpv6: {
                    label: 'Require IPv6',
                    hint: 'When on, nodes need an IPv6 default route and a global address, and IPv6 forwarding and NAT rules are installed',
                },
            },
        },
    },
} satisfies AppsMessages
