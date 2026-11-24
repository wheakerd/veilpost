// Certificates and DNS provider credentials (views CertificatesView and
// DnsProvidersView). Provider and CA brand names stay untranslated.
const certs = {
    certificates: {
        description:
            '通过 ACME 协议与 DNS-01 验证申请 TLS 证书，到期前自动续期',
        request: '申请证书',
        listTitle: '证书列表',
        listLabel: '证书',
        columns: { domains: '域名', status: '状态', actions: '操作' },
        count: '{count} 张证书',
        form: {
            description:
                '签发在后台进行，提交后列表会显示“签发中”。请先在“DNS 服务商”中添加可以管理这些域名解析的凭据。',
            domains: '域名',
            domainsHint:
                '每行一个，或用逗号、空格分隔；支持 *.example.com 通配符，最多 100 个。',
            credential: 'DNS 服务商凭据',
            credentialPlaceholder: '请选择凭据',
            noCredentials: '还没有 DNS 服务商凭据',
            addCredential: '去添加凭据',
            directory: 'ACME 服务',
            directoryUrl: 'ACME 目录地址',
            directoryUrlHint:
                '只接受 https:// 地址，例如 ZeroSSL 或 Google Trust Services 的目录地址',
            eabTitle: '外部账户绑定（EAB，可选）',
            eabHint:
                '服务商要求时填写；仅在首次注册 ACME 账户时使用，注册成功后不再保存。',
            eabKeyId: 'EAB Key ID',
            eabHmacKey: 'EAB HMAC Key',
            email: '联系邮箱（可选）',
            emailHint: '用于 ACME 账户联系；同一 ACME 服务和邮箱共用一个账户。',
            terms: '提交即表示代表你同意所选 ACME 服务的服务条款',
            submit: '提交申请',
            missingDomains: '请至少输入一个域名。',
            missingCredential: '请选择 DNS 服务商凭据。',
            missingDirectoryUrl: '请输入以 https:// 开头的 ACME 目录地址。',
            incompleteEab: 'EAB Key ID 与 HMAC Key 需要同时填写。',
        },
        directories: {
            letsencrypt: "Let's Encrypt",
            letsencryptStaging: "Let's Encrypt 测试环境",
            custom: '自定义 ACME 目录',
        },
        state: {
            pending: '签发中',
            valid: '有效',
            failed: '签发失败',
            expired: '已过期',
            renewing: '续期中',
        },
        meta: {
            issuer: '颁发机构',
            notAfter: '到期时间',
            renewAt: '计划续期',
            directory: 'ACME 服务',
            credential: 'DNS 凭据',
            email: '联系邮箱',
            none: '—',
        },
        moreDomains: '等 {count} 个域名',
        lastError: '最近错误（{time}）：{message}',
        issueErrors: {
            CREDENTIAL_UNAVAILABLE:
                'DNS 服务商凭据不可用，请检查或重新填写凭据。',
            ACME_UNREACHABLE: '无法连接 ACME 服务，请检查目录地址与网络。',
            ACME_ACCOUNT_FAILED:
                'ACME 账户注册失败；如服务商要求 EAB，请填写后重新申请。',
            ACME_RATE_LIMITED:
                'ACME 服务拒绝了请求：已达到签发频率限制，稍后会自动重试。',
            ACME_REJECTED: 'ACME 服务拒绝签发，请核对域名与 DNS 记录。',
            DNS_PROVIDER_FAILED:
                '无法通过 DNS 服务商创建验证记录，请确认凭据有编辑该域名解析的权限。',
            DNS_PROPAGATION_TIMEOUT:
                '验证记录在限定时间内未在权威 DNS 上生效。',
            ISSUANCE_FAILED: '签发未完成，请稍后重新签发。',
        },
        reissue: '重新签发',
        download: '下载证书链',
        delete: {
            open: '删除',
            description:
                '删除后，该证书与其私钥将从控制台移除，无法恢复；正在进行的签发会停止',
            submit: '确认删除',
            done: '证书已删除',
        },
        requested: '已提交申请，正在后台签发',
        reissued: '已开始重新签发',
        downloaded: '证书链已下载。私钥不提供浏览器下载。',
        unavailable: {
            stale: '暂时无法刷新证书状态，下面是上次读取的结果。',
            empty: '暂时无法读取证书列表，请稍后刷新页面。',
        },
        empty: {
            title: '还没有证书',
            first: '申请证书后，这里会显示签发状态、颁发机构和到期时间',
            second: '控制台会在到期前 30 天自动续期',
        },
        note: '私钥使用控制台主密钥加密保存，不提供浏览器下载；后续由节点代理程序按需获取',
    },
    dns: {
        description:
            '保存用于 DNS-01 验证的服务商凭据。密钥只写不读，界面只显示是否已设置。',
        add: '添加凭据',
        listTitle: '凭据列表',
        listLabel: 'DNS 服务商凭据',
        count: '{count} 个凭据',
        providers: {
            cloudflare: 'Cloudflare',
            alidns: '阿里云 DNS',
            tencentcloud: '腾讯云 DNSPod',
            route53: 'AWS Route 53',
        },
        fields: {
            apiToken: 'API Token',
            accessKeyId: 'Access Key ID',
            accessKeySecret: 'AccessKey Secret',
            secretId: 'SecretId',
            secretKey: 'SecretKey',
            secretAccessKey: 'Secret Access Key',
            region: '区域',
            hostedZoneId: 'Hosted Zone ID（可选）',
        },
        hints: {
            cloudflare: '需要 Zone:Read 与 DNS:Edit 权限的 API Token',
            alidns: '建议使用仅有云解析权限的 RAM 子账号 AccessKey',
            tencentcloud: '建议使用仅有 DNSPod 权限的子账号密钥',
            route53: '需要 Route 53 的读取与修改记录权限；区域一般为 us-east-1',
        },
        form: {
            createTitle: '添加 DNS 服务商凭据',
            editTitle: '修改凭据“{name}”',
            description: '保存后密钥不会再显示。请输入当前密码确认。',
            editDescription: '留空的密钥保持不变。请输入当前密码确认。',
            name: '名称',
            nameHint: '1–64 个字符，仅用于在控制台中区分凭据',
            provider: '服务商',
            keepValue: '已设置，留空保持不变',
            submitCreate: '保存凭据',
            submitEdit: '保存修改',
            missing: '请填写名称、全部必填密钥和当前密码。',
            missingPassword: '请输入当前密码。',
            created: '凭据“{name}”已保存',
            updated: '凭据“{name}”已更新',
        },
        status: {
            set: '已设置',
            unset: '未设置',
        },
        meta: {
            updatedAt: '最后修改',
            usage: '使用情况',
            unused: '未被证书使用',
            usedBy: '{count} 张证书使用',
        },
        verify: {
            open: '验证凭据',
            done: '凭据“{name}”可用：服务商接受了只读查询',
        },
        edit: '修改',
        delete: {
            open: '删除',
            description:
                '删除后无法恢复。被证书使用的凭据不能删除。请输入当前密码确认。',
            submit: '确认删除',
            done: '凭据“{name}”已删除',
        },
        unavailable: {
            stale: '暂时无法刷新凭据列表，下面是上次读取的结果。',
            empty: '暂时无法读取凭据列表，请稍后刷新页面。',
        },
        empty: {
            title: '还没有 DNS 服务商凭据',
            first: '支持 Cloudflare、阿里云 DNS、腾讯云 DNSPod 与 AWS Route 53',
            second: '添加凭据后即可申请证书；验证凭据只做只读查询，不会创建解析记录。',
        },
        note: '凭据使用控制台主密钥加密保存，接口与日志都不会返回明文',
    },
}

export type CertsMessages = typeof certs
export default certs
