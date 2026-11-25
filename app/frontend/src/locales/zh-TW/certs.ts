import type { CertsMessages } from '../zh-CN/certs'

export default {
    certificates: {
        description:
            '透過 ACME 協定與 DNS-01 驗證申請 TLS 憑證，到期前自動續期',
        request: '申請憑證',
        listTitle: '憑證列表',
        listLabel: '憑證',
        columns: { domains: '網域', status: '狀態', actions: '操作' },
        count: '{count} 張憑證',
        form: {
            description:
                '簽發在背景進行，送出後列表會顯示「簽發中」。請先在「DNS 服務商」中新增可以管理這些網域解析的憑據。',
            domains: '網域',
            domainsHint:
                '每行一個，或以逗號、空格分隔；支援 *.example.com 萬用字元，最多 100 個。',
            credential: 'DNS 服務商憑據',
            credentialPlaceholder: '請選擇憑據',
            noCredentials: '尚未新增 DNS 服務商憑據',
            addCredential: '前往新增憑據',
            directory: 'ACME 服務',
            directoryUrl: 'ACME 目錄位址',
            directoryUrlHint:
                '只接受 https:// 位址，例如 ZeroSSL 或 Google Trust Services 的目錄位址',
            eabTitle: '外部帳戶綁定（EAB，選填）',
            eabHint:
                '服務商要求時填寫；僅在首次註冊 ACME 帳戶時使用，註冊成功後不再保存。',
            eabKeyId: 'EAB Key ID',
            eabHmacKey: 'EAB HMAC Key',
            email: '聯絡信箱（選填）',
            emailHint: '用於 ACME 帳戶聯絡；同一 ACME 服務與信箱共用一個帳戶。',
            terms: '送出即表示代表你同意所選 ACME 服務的服務條款',
            submit: '送出申請',
            missingDomains: '請至少輸入一個網域。',
            missingCredential: '請選擇 DNS 服務商憑據。',
            missingDirectoryUrl: '請輸入以 https:// 開頭的 ACME 目錄位址。',
            incompleteEab: 'EAB Key ID 與 HMAC Key 需要同時填寫。',
        },
        directories: {
            letsencrypt: "Let's Encrypt",
            letsencryptStaging: "Let's Encrypt 測試環境",
            custom: '自訂 ACME 目錄',
        },
        state: {
            pending: '簽發中',
            valid: '有效',
            failed: '簽發失敗',
            expired: '已過期',
            renewing: '續期中',
        },
        meta: {
            issuer: '簽發機構',
            notAfter: '到期時間',
            renewAt: '預計續期',
            directory: 'ACME 服務',
            credential: 'DNS 憑據',
            email: '聯絡信箱',
            none: '—',
        },
        moreDomains: '等 {count} 個網域',
        lastError: '最近錯誤（{time}）：{message}',
        issueErrors: {
            CREDENTIAL_UNAVAILABLE:
                'DNS 服務商憑據無法使用，請檢查或重新填寫憑據。',
            ACME_UNREACHABLE: '無法連線至 ACME 服務，請檢查目錄位址與網路。',
            ACME_ACCOUNT_FAILED:
                'ACME 帳戶註冊失敗；若服務商要求 EAB，請填寫後重新申請。',
            ACME_RATE_LIMITED:
                'ACME 服務拒絕了請求：已達簽發頻率限制，稍後會自動重試。',
            ACME_REJECTED: 'ACME 服務拒絕簽發，請核對網域與 DNS 記錄。',
            DNS_PROVIDER_FAILED:
                '無法透過 DNS 服務商建立驗證記錄，請確認憑據具有編輯該網域解析的權限。',
            DNS_PROPAGATION_TIMEOUT:
                '驗證記錄未在限定時間內於權威 DNS 上生效。',
            ISSUANCE_FAILED: '簽發未完成，請稍後重新簽發。',
        },
        reissue: '重新簽發',
        download: '下載憑證鏈',
        delete: {
            open: '刪除',
            description:
                '刪除後，該憑證與其私鑰將從主控台移除，無法復原；進行中的簽發會停止',
            submit: '確認刪除',
            done: '憑證已刪除',
        },
        requested: '已送出申請，正在背景簽發',
        reissued: '已開始重新簽發',
        downloaded: '憑證鏈已下載。私鑰不提供瀏覽器下載。',
        unavailable: {
            stale: '暫時無法重新整理憑證狀態，以下是上次讀取的結果。',
            empty: '暫時無法讀取憑證列表，請稍後重新整理頁面。',
        },
        empty: {
            title: '尚無憑證',
            first: '申請憑證後，這裡會顯示簽發狀態、簽發機構與到期時間',
            second: '主控台會在到期前 30 天自動續期',
        },
        note: '私鑰以主控台主金鑰加密保存，不提供瀏覽器下載；後續由節點代理程式依需要取得',
    },
    dns: {
        description:
            '保存用於 DNS-01 驗證的服務商憑據。金鑰只寫不讀，介面只顯示是否已設定。',
        add: '新增憑據',
        listTitle: '憑據列表',
        listLabel: 'DNS 服務商憑據',
        count: '{count} 個憑據',
        providers: {
            cloudflare: 'Cloudflare',
            alidns: '阿里雲 DNS',
            tencentcloud: '騰訊雲 DNSPod',
            route53: 'AWS Route 53',
        },
        fields: {
            apiToken: 'API Token',
            accessKeyId: 'Access Key ID',
            accessKeySecret: 'AccessKey Secret',
            secretId: 'SecretId',
            secretKey: 'SecretKey',
            secretAccessKey: 'Secret Access Key',
            region: '區域',
            hostedZoneId: 'Hosted Zone ID（選填）',
        },
        hints: {
            cloudflare: '需要具備 Zone:Read 與 DNS:Edit 權限的 API Token',
            alidns: '建議使用僅有雲解析權限的 RAM 子帳號 AccessKey',
            tencentcloud: '建議使用僅有 DNSPod 權限的子帳號金鑰',
            route53: '需要 Route 53 的讀取與修改記錄權限；區域一般為 us-east-1',
        },
        form: {
            createTitle: '新增 DNS 服務商憑據',
            editTitle: '修改憑據「{name}」',
            description: '保存後金鑰不會再顯示。請輸入目前密碼確認。',
            editDescription: '留空的金鑰保持不變。請輸入目前密碼確認。',
            name: '備註（選填）',
            nameHint: '最多 64 個字元；不填寫時顯示服務商名稱。',
            provider: '服務商',
            keepValue: '已設定，留空保持不變',
            submitCreate: '保存憑據',
            submitEdit: '保存修改',
            missing: '請填寫全部必填金鑰與目前密碼。',
            missingPassword: '請輸入目前密碼。',
            created: '憑據「{name}」已保存',
            updated: '憑據「{name}」已更新',
        },
        status: {
            set: '已設定',
            unset: '未設定',
        },
        meta: {
            updatedAt: '最後修改',
            usage: '使用情況',
            unused: '未被憑證使用',
            usedBy: '{count} 張憑證使用',
        },
        verify: {
            open: '驗證憑據',
            done: '憑據「{name}」可用：服務商接受了唯讀查詢',
        },
        edit: '修改',
        delete: {
            open: '刪除',
            description:
                '刪除後無法復原。被憑證使用的憑據不能刪除。請輸入目前密碼確認。',
            submit: '確認刪除',
            done: '憑據「{name}」已刪除',
        },
        unavailable: {
            stale: '暫時無法重新整理憑據列表，以下是上次讀取的結果。',
            empty: '暫時無法讀取憑據列表，請稍後重新整理頁面。',
        },
        empty: {
            title: '尚無 DNS 服務商憑據',
            first: '支援 Cloudflare、阿里雲 DNS、騰訊雲 DNSPod 與 AWS Route 53',
            second: '新增憑據後即可申請憑證；驗證憑據只做唯讀查詢，不會建立解析記錄。',
        },
        note: '憑據以主控台主金鑰加密保存，介面與日誌都不會回傳明文',
    },
} satisfies CertsMessages
