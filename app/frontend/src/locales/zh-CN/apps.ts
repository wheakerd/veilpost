// The software store and installed app pages. apps.catalog holds the names,
// descriptions and setting labels that the backend manifests reference by
// key (app/backend/internal/apps/catalog/*.json).
const apps = {
    store: {
        description: '启用 VPN 软件后，侧栏会出现它的配置与管理页面',
        listTitle: '软件目录',
        listLabel: '软件',
        installedCount: '已安装 {count} 个',
        builtIn:
            '目录随控制台版本发布，镜像按摘要固定；不支持远程软件源或上传自定义包。',
        version: '版本 {version}',
        protocols: '协议',
        ports: '端口',
        image: '镜像',
        randomPort: '部署时按节点随机',
        localBuild: '由本仓库 {path} 构建，尚无发布地址',
        state: {
            notInstalled: '未安装',
            installed: '已安装',
            needsSettings: '待配置',
        },
        install: '安装',
        installedNotice: '已安装“{name}”，侧栏中已出现它的页面。',
        manage: '配置与管理',
        unavailable: {
            stale: '暂时无法刷新软件目录，下面是上次读取的结果。',
            empty: '暂时无法读取软件目录，请稍后刷新页面。',
        },
    },
    uninstall: {
        open: '卸载',
        description:
            '卸载会删除该软件的全部配置，包括已保存的敏感值，无法恢复。请输入当前密码确认。',
        submit: '确认卸载',
        done: '“{name}”已卸载，配置已删除',
        missingPassword: '请输入当前密码。',
    },
    app: {
        notInstalled: {
            title: '“{name}”尚未安装',
            description: '在软件商店中安装后，这里会显示它的配置与管理页面',
        },
        notFound: {
            title: '软件目录中没有这个软件',
            description: '请检查地址，或在软件商店中查看可用的软件',
        },
        toStore: '前往软件商店',
        unavailable: '暂时无法读取该软件，请稍后刷新页面。',
        settings: {
            title: '配置',
            description:
                '配置项由软件清单定义并在服务器端校验。敏感项只写不读：保存后只显示是否已设置。',
            complete: '配置完整',
            incomplete: '待配置：还有必填项未填写',
            required: '必填',
            secretSet: '已设置',
            secretUnset: '未设置',
            secretKeep: '留空则保留已保存的值',
            secretClear: '清除已保存的值',
            secretClearing: '保存后会清除已保存的值',
            secretUndo: '撤销清除',
            enabled: '开启',
            save: '保存配置',
            saved: '配置已保存',
            invalidField: '“{field}”的值无效，请按说明修改后重试。',
            requiredField: '请填写“{field}”。',
            updatedAt: '最后修改：{time}',
        },
        management: {
            title: '管理',
            version: '版本',
            installedAt: '安装时间',
            image: '镜像',
            platforms: '平台',
            source: '上游源码',
            deployTitle: '部署到节点',
            deployUnavailable:
                '尚未开放：节点代理程序需要先支持配置下发（节点第 2 阶段）。在此之前，任何节点都不会运行该软件。',
            deployAction: '部署到节点',
        },
    },
    catalog: {
        xray: {
            name: 'Xray-core',
            description:
                '两类 TCP 入口：VLESS + REALITY + Vision 与 VLESS + XHTTP + REALITY，借用真实站点的 TLS 握手',
            settings: {
                visionPort: {
                    label: 'Vision 入口端口',
                    hint: 'TCP 端口，默认 443。',
                },
                visionTarget: {
                    label: 'Vision 借用站点（target）',
                    hint: '域名或“域名:端口”，不写端口时为 443。站点需支持 TLS 1.3、HTTP/2 且无跳转；没有默认值，必须填写。',
                },
                visionServerName: {
                    label: 'Vision 服务器名称（serverName）',
                    hint: '客户端发送的 SNI；留空则使用借用站点的域名。',
                },
                xhttpPort: {
                    label: 'XHTTP 入口端口',
                    hint: 'TCP 端口；留空则部署时按节点随机选择高端口。',
                },
                xhttpTarget: {
                    label: 'XHTTP 借用站点（target）',
                    hint: '可与 Vision 入口不同，格式与要求相同；没有默认值，必须填写。',
                },
                xhttpServerName: {
                    label: 'XHTTP 服务器名称（serverName）',
                    hint: '留空则使用借用站点的域名',
                },
                xhttpPath: {
                    label: 'XHTTP 路径',
                    hint: '以 / 开头，2–64 个字符，只能包含字母、数字和 . _ ~ -；留空则部署时按节点随机生成。',
                },
                xhttpMode: {
                    label: 'XHTTP 模式',
                    hint: '显式指定，不使用 auto',
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
                '官方服务端，UDP（QUIC）入口。伪装方式应答 HTTP/3 探测；Salamander 混淆隐藏 QUIC 特征。',
            settings: {
                port: {
                    label: 'UDP 端口',
                    hint: '默认 443，与 TCP 443 入口互不冲突',
                },
                obfuscation: {
                    label: '伪装方式',
                    hint: '伪装方式抗主动探测；Salamander 看起来是随机 UDP，但无法应答探测。',
                    options: {
                        masquerade: '伪装为 HTTP/3 站点',
                        salamander: 'Salamander 混淆',
                    },
                },
                masqueradeUrl: {
                    label: '伪装站点地址',
                    hint: '未认证的请求由这个 https 站点应答；选择伪装方式时必须填写。',
                },
                salamanderPassword: {
                    label: 'Salamander 混淆密码',
                    hint: '16–128 个字符；选择 Salamander 时必须设置。保存后不会再显示。',
                },
            },
        },
        amneziawg: {
            name: 'AmneziaWG',
            description:
                'UDP 入口，使用 AmneziaWG 2.0 混淆参数（按节点随机生成）。镜像由本仓库构建。',
            settings: {
                port: {
                    label: 'UDP 端口',
                    hint: 'UDP 端口；留空则部署时按节点随机选择高端口。UDP 443 留给 Hysteria 2。',
                },
                mtu: {
                    label: 'MTU',
                    hint: '1280–1420，默认 1280。',
                },
                requireIpv6: {
                    label: '要求 IPv6',
                    hint: '开启后，节点必须有 IPv6 默认路由和全局地址，并安装 IPv6 转发与 NAT 规则',
                },
            },
        },
    },
}

export type AppsMessages = typeof apps
export default apps
