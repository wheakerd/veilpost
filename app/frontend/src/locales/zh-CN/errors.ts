// Keys are the error codes returned by the backend (docs/auth-api.md,
// docs/nodes-api.md, docs/apps-api.md). Unknown codes are shown as
// REQUEST_FAILED.
const errors = {
    INVALID_CREDENTIALS: '账号或密码不正确，请重新输入。',
    INVALID_PASSWORD: '当前密码不正确，请重新输入。',
    WEAK_PASSWORD: '新密码至少需要 12 个字符，且不超过 256 字节。',
    PASSWORD_REUSED: '新密码不能与当前密码相同。',
    INVALID_CODE:
        '验证码不正确、已使用或已失效，请等待验证器中的下一组验证码。',
    INVALID_RECOVERY_CODE: '恢复码无效或已使用，请尝试其他恢复码。',
    CHALLENGE_EXPIRED: '本次验证已过期，请重新登录。',
    SETUP_EXPIRED: '本次绑定已过期，请重新开始设置。',
    SESSION_EXPIRED: '登录状态已失效，请重新登录。',
    TOO_MANY_ATTEMPTS: '尝试次数过多，请稍后再试。',
    SERVICE_UNAVAILABLE: '认证服务暂不可用，请稍后重试。',
    REQUEST_FAILED: '操作未完成，请重试。',
    INVALID_NAME: '节点名称需为 1–64 个字符，且不能包含换行等控制字符。',
    NODE_NOT_FOUND: '节点不存在，请刷新列表后重试。',
    NODE_NOT_PENDING: '节点已完成注册或已吊销，无法重新签发注册令牌。',
    NODE_NOT_REVOKED: '只能删除已吊销的节点，请先吊销。',
    APP_NOT_FOUND: '软件目录中没有这个软件。',
    APP_NOT_INSTALLED: '该软件尚未安装，请先在软件商店中安装。',
    APP_ALREADY_INSTALLED: '该软件已经安装。',
    APP_DEPLOYED: '该软件已部署到节点，请先从所有节点移除后再卸载。',
    INVALID_SETTING: '有配置项的值无效，请检查后重试。',
    SETTING_REQUIRED: '还有必填的配置项未填写。',
    UNKNOWN_SETTING: '配置中包含该软件不支持的项目，请刷新页面后重试。',
    INVALID_CREDENTIAL_NAME:
        '凭据名称需为 1–64 个字符，且不能包含换行等控制字符。',
    INVALID_PROVIDER: '不支持该 DNS 服务商。',
    INVALID_SECRETS: '凭据字段不完整或格式不正确，请检查后重试。',
    CREDENTIAL_NOT_FOUND: 'DNS 服务商凭据不存在，请刷新列表后重试。',
    CREDENTIAL_IN_USE: '该凭据正被证书使用，请先删除相关证书。',
    CREDENTIAL_UNREADABLE: '无法读取已保存的凭据，请重新填写全部密钥。',
    CREDENTIAL_REJECTED: '服务商拒绝了该凭据，请检查密钥与权限。',
    PROVIDER_UNREACHABLE: '暂时无法连接 DNS 服务商，请稍后重试。',
    INVALID_DOMAIN:
        '域名格式不正确：只接受合法的 DNS 名称，通配符只能写作 *.example.com。',
    TOO_MANY_DOMAINS: '一张证书最多包含 100 个域名。',
    INVALID_DIRECTORY: 'ACME 目录地址无效，只接受 https:// 地址。',
    INVALID_EMAIL: '联系邮箱格式不正确。',
    INVALID_EAB:
        'EAB 信息无效：仅自定义 ACME 目录可用，且 HMAC Key 需为 base64url 编码。',
    CERTIFICATE_NOT_FOUND: '证书不存在，请刷新列表后重试。',
    CERTIFICATE_NOT_ISSUED: '证书尚未签发，暂时无法下载。',
}

export type ErrorMessages = typeof errors
export default errors
