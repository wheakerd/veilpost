import type { AppsMessages } from '../zh-CN/apps'

export default {
    store: {
        description: '啟用 VPN 軟體後，側邊欄會出現它的設定與管理頁面',
        listTitle: '軟體目錄',
        listLabel: '軟體',
        installedCount: '已安裝 {count} 個',
        builtIn:
            '目錄隨控制台版本發布，映像檔以摘要固定；不支援遠端軟體來源或上傳自訂套件。',
        version: '版本 {version}',
        protocols: '協定',
        ports: '連接埠',
        image: '映像檔',
        randomPort: '部署時依節點隨機',
        localBuild: '由本儲存庫 {path} 建置，尚無發布位址',
        state: {
            notInstalled: '未安裝',
            installed: '已安裝',
            needsSettings: '待設定',
        },
        install: '安裝',
        installedNotice: '已安裝「{name}」，側邊欄中已出現它的頁面。',
        manage: '設定與管理',
        unavailable: {
            stale: '暫時無法重新整理軟體目錄，以下是上次讀取的結果。',
            empty: '暫時無法讀取軟體目錄，請稍後重新整理頁面。',
        },
    },
    uninstall: {
        open: '解除安裝',
        description:
            '解除安裝會刪除該軟體的全部設定，包括已儲存的敏感值，無法復原。請輸入目前的密碼確認。',
        submit: '確認解除安裝',
        done: '「{name}」已解除安裝，設定已刪除',
        missingPassword: '請輸入目前的密碼。',
    },
    app: {
        notInstalled: {
            title: '「{name}」尚未安裝',
            description: '在軟體商店中安裝後，這裡會顯示它的設定與管理頁面',
        },
        notFound: {
            title: '軟體目錄中沒有這個軟體',
            description: '請檢查網址，或在軟體商店中查看可用的軟體',
        },
        toStore: '前往軟體商店',
        unavailable: '暫時無法讀取該軟體，請稍後重新整理頁面。',
        settings: {
            title: '設定',
            description:
                '設定項目由軟體清單定義並在伺服器端驗證。敏感項目只寫不讀：儲存後只顯示是否已設定。',
            complete: '設定完整',
            incomplete: '待設定：還有必填項目未填寫',
            required: '必填',
            secretSet: '已設定',
            secretUnset: '未設定',
            secretKeep: '留空則保留已儲存的值',
            secretClear: '清除已儲存的值',
            secretClearing: '儲存後會清除已儲存的值',
            secretUndo: '取消清除',
            enabled: '開啟',
            save: '儲存設定',
            saved: '設定已儲存',
            invalidField: '「{field}」的值無效，請依說明修改後重試。',
            requiredField: '請填寫「{field}」。',
            updatedAt: '最後修改：{time}',
        },
        management: {
            title: '管理',
            version: '版本',
            installedAt: '安裝時間',
            image: '映像檔',
            platforms: '平台',
            source: '上游原始碼',
            deployTitle: '部署到節點',
            deployUnavailable:
                '尚未開放：節點代理程式需要先支援設定下發（節點第 2 階段）。在此之前，任何節點都不會執行該軟體。',
            deployAction: '部署到節點',
        },
    },
    catalog: {
        xray: {
            name: 'Xray-core',
            description:
                '兩類 TCP 入口：VLESS + REALITY + Vision 與 VLESS + XHTTP + REALITY，借用真實網站的 TLS 交握',
            settings: {
                visionPort: {
                    label: 'Vision 入口連接埠',
                    hint: 'TCP 連接埠，預設 443。',
                },
                visionTarget: {
                    label: 'Vision 借用網站（target）',
                    hint: '網域或「網域:連接埠」，未填連接埠時為 443。網站需支援 TLS 1.3、HTTP/2 且無重新導向；沒有預設值，必須填寫。',
                },
                visionServerName: {
                    label: 'Vision 伺服器名稱（serverName）',
                    hint: '用戶端送出的 SNI；留空則使用借用網站的網域。',
                },
                xhttpPort: {
                    label: 'XHTTP 入口連接埠',
                    hint: 'TCP 連接埠；留空則部署時依節點隨機選擇高位連接埠。',
                },
                xhttpTarget: {
                    label: 'XHTTP 借用網站（target）',
                    hint: '可與 Vision 入口不同，格式與要求相同；沒有預設值，必須填寫。',
                },
                xhttpServerName: {
                    label: 'XHTTP 伺服器名稱（serverName）',
                    hint: '留空則使用借用網站的網域',
                },
                xhttpPath: {
                    label: 'XHTTP 路徑',
                    hint: '以 / 開頭，2–64 個字元，只能包含字母、數字和 . _ ~ -；留空則部署時依節點隨機產生。',
                },
                xhttpMode: {
                    label: 'XHTTP 模式',
                    hint: '明確指定，不使用 auto',
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
                '官方伺服器，UDP（QUIC）入口。偽裝方式回應 HTTP/3 探測；Salamander 混淆隱藏 QUIC 特徵。',
            settings: {
                port: {
                    label: 'UDP 連接埠',
                    hint: '預設 443，與 TCP 443 入口互不衝突',
                },
                obfuscation: {
                    label: '偽裝方式',
                    hint: '偽裝方式可抵抗主動探測；Salamander 看起來是隨機 UDP，但無法回應探測。',
                    options: {
                        masquerade: '偽裝為 HTTP/3 網站',
                        salamander: 'Salamander 混淆',
                    },
                },
                masqueradeUrl: {
                    label: '偽裝網站位址',
                    hint: '未驗證的請求由這個 https 網站回應；選擇偽裝方式時必須填寫。',
                },
                salamanderPassword: {
                    label: 'Salamander 混淆密碼',
                    hint: '16–128 個字元；選擇 Salamander 時必須設定。儲存後不會再顯示。',
                },
            },
        },
        amneziawg: {
            name: 'AmneziaWG',
            description:
                'UDP 入口，使用 AmneziaWG 2.0 混淆參數（依節點隨機產生）。映像檔由本儲存庫建置。',
            settings: {
                port: {
                    label: 'UDP 連接埠',
                    hint: 'UDP 連接埠；留空則部署時依節點隨機選擇高位連接埠。UDP 443 保留給 Hysteria 2。',
                },
                mtu: {
                    label: 'MTU',
                    hint: '1280–1420，預設 1280。',
                },
                requireIpv6: {
                    label: '要求 IPv6',
                    hint: '開啟後，節點必須有 IPv6 預設路由和全域位址，並安裝 IPv6 轉送與 NAT 規則',
                },
            },
        },
    },
} satisfies AppsMessages
