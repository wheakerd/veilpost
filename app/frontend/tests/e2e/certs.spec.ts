import { randomBytes } from 'node:crypto'
import { createServer } from 'node:net'
import { expect, type Locator, type Page } from '@playwright/test'
import {
    test,
    signIn,
    login,
    headers,
    noStoredSecrets,
    type Account,
} from './real-auth'
import { choose, chosen, tick } from './element'

// These tests drive the real Go backend. They never reach a public ACME
// service or DNS provider: certificate requests use a custom directory on a
// closed loopback port, so the real background job fails with
// ACME_UNREACHABLE, and credentials are never verified against a provider.
const origin = 'http://127.0.0.1:4173'

function unique(prefix: string) {
    return `${prefix}-${randomBytes(4).toString('hex')}`
}

function secret() {
    return `e2e-secret-${randomBytes(12).toString('hex')}`
}

// A loopback port with nothing listening on it.
async function closedDirectory() {
    const server = createServer()
    await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
    const { port } = server.address() as { port: number }
    await new Promise((resolve) => server.close(resolve))
    return `https://127.0.0.1:${port}/directory`
}

async function createCredential(
    page: Page,
    account: Account,
    name: string,
    token: string,
) {
    const response = await page.request.post(`${origin}/api/dns-credentials`, {
        headers,
        data: {
            name,
            provider: 'cloudflare',
            secrets: { apiToken: token },
            password: account.password,
        },
    })
    expect(response.status()).toBe(201)
    return (await response.json()).credential.id as string
}

async function requestCertificate(
    page: Page,
    credentialId: string,
    domain: string,
) {
    const response = await page.request.post(`${origin}/api/certificates`, {
        headers,
        data: {
            domains: [domain],
            dnsCredentialId: credentialId,
            directory: 'custom',
            directoryUrl: await closedDirectory(),
            email: '',
            eab: { keyId: '', hmacKey: '' },
        },
    })
    expect(response.status()).toBe(202)
    return (await response.json()).certificate.id as string
}

function credentialRow(page: Page, id: string): Locator {
    return page.locator(`li[data-credential-id="${id}"]`)
}

async function chooseCredential(page: Page, id: string, label: string) {
    const select = chosen(page, page.getByLabel('DNS 服务商凭据'))
    await select.locator('.el-select__wrapper').click()
    const option = page
        .getByRole('option')
        .and(page.locator(`[data-credential-id="${id}"]`))
    await expect(option).toHaveText(label)
    await option.click()
    await expect(
        select.locator('.el-select__selected-item').filter({ hasText: label }),
    ).toHaveText(label)
}

async function saveNewCredential(page: Page) {
    const response = page.waitForResponse(
        (response) =>
            response.url() === `${origin}/api/dns-credentials` &&
            response.request().method() === 'POST',
    )
    await page.getByRole('button', { name: '保存凭据' }).click()
    const saved = await response
    expect(saved.status()).toBe(201)
    return (await saved.json()).credential as { id: string; name: string }
}

function certificateRow(page: Page, domain: string): Locator {
    return page.locator('tr[data-certificate-id]').filter({ hasText: domain })
}

async function noHorizontalOverflow(page: Page) {
    expect(
        await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
    ).toBe(true)
}

test('DNS credentials are write-only and never shown again', async ({
    page,
    account,
}) => {
    await signIn(page, account)
    await page.goto('/dns-providers')
    await expect(
        page.getByRole('heading', { name: 'DNS 服务商', level: 1 }),
    ).toBeVisible()
    const name = unique('e2e-cf')
    const token = secret()

    await page.getByRole('button', { name: '添加凭据' }).click()
    await page.getByLabel('备注（可选）', { exact: true }).fill(name)
    await expect(
        chosen(page, page.getByLabel('服务商', { exact: true })),
    ).toContainText('Cloudflare')
    await page.getByLabel('API Token').fill(token)
    await page.getByLabel('当前密码', { exact: true }).fill(account.password)
    const { id: credentialId } = await saveNewCredential(page)
    await expect(page.getByText(`凭据“${name}”已保存`)).toBeVisible()

    const row = credentialRow(page, credentialId)
    await expect(row).toContainText('Cloudflare')
    await expect(row.locator('[data-field="apiToken"]')).toContainText('已设置')
    await expect(row).toContainText('未被证书使用')
    await expect(page.locator('body')).not.toContainText(token)
    await noHorizontalOverflow(page)

    // Editing never loads the stored value; a blank field keeps it.
    await row.getByRole('button', { name: '修改' }).click()
    const tokenInput = page.getByLabel('API Token')
    await expect(tokenInput).toHaveValue('')
    await expect(tokenInput).toHaveAttribute(
        'placeholder',
        '已设置，留空保持不变',
    )
    await expect(tokenInput).toHaveAttribute('type', 'password')
    await page
        .getByLabel('备注（可选）', { exact: true })
        .fill(`${name}-renamed`)
    await page
        .getByLabel('当前密码', { exact: true })
        .fill('wrong-password-value')
    await page.getByRole('button', { name: '保存修改' }).click()
    await expect(page.getByText('当前密码不正确，请重新输入。')).toBeVisible()
    await expect(page.getByLabel('当前密码', { exact: true })).toHaveValue('')
    await page.getByLabel('当前密码', { exact: true }).fill(account.password)
    await page.getByRole('button', { name: '保存修改' }).click()
    await expect(page.getByText(`凭据“${name}-renamed”已更新`)).toBeVisible()
    await expect(
        credentialRow(page, credentialId).locator('[data-field="apiToken"]'),
    ).toContainText('已设置')

    // Neither the API nor browser storage holds the token or the password.
    const listed = await page.request.get(`${origin}/api/dns-credentials`)
    expect(await listed.text()).not.toContain(token)
    await noStoredSecrets(page, account)
    const storage = await page.evaluate(() =>
        JSON.stringify({ ...localStorage, ...sessionStorage }),
    )
    expect(storage).not.toContain(token)

    // Deleting requires the password.
    await credentialRow(page, credentialId)
        .getByRole('button', { name: '删除' })
        .click()
    await page.getByLabel('当前密码', { exact: true }).fill(account.password)
    await page.getByRole('button', { name: '确认删除' }).click()
    await expect(page.getByText(`凭据“${name}-renamed”已删除`)).toBeVisible()
    await expect(credentialRow(page, credentialId)).toHaveCount(0)
})

for (const [kind, initialNote, provider, providerLabel] of [
    ['empty', '', 'cloudflare', 'Cloudflare'],
    ['whitespace', '  　 ', 'alidns', '阿里云 DNS'],
] as const) {
    test(`DNS ${kind} notes stay empty and share provider fallback labels`, async ({
        page,
        account,
    }) => {
        test.setTimeout(60_000)
        await signIn(page, account)
        await page.goto('/dns-providers')
        await page.getByRole('button', { name: '添加凭据' }).click()
        const note = page.getByLabel('备注（可选）', { exact: true })
        await note.fill(initialNote)
        await expect(
            page.getByText('最多 64 个字符；不填写时显示服务商名称。'),
        ).toBeVisible()
        if (provider === 'alidns') {
            await choose(
                page,
                page.getByLabel('服务商', { exact: true }),
                providerLabel,
            )
            await page
                .getByLabel('Access Key ID', { exact: true })
                .fill(secret())
            await page
                .getByLabel('AccessKey Secret', { exact: true })
                .fill(secret())
        } else {
            await page.getByLabel('API Token').fill(secret())
        }
        await page
            .getByLabel('当前密码', { exact: true })
            .fill(account.password)
        const credential = await saveNewCredential(page)
        expect(credential.name).toBe('')
        await expect(
            page.getByText(`凭据“${providerLabel}”已保存`),
        ).toBeVisible()
        const row = credentialRow(page, credential.id)
        await expect(row.locator('.node-name strong')).toHaveText(providerLabel)
        await expect(row.locator('.node-name small')).toHaveCount(0)
        await expect(
            row.getByRole('list', { name: providerLabel, exact: true }),
        ).toBeVisible()

        // Reloading must not replace the empty stored note with a translated name.
        await page.reload()
        await row.getByRole('button', { name: '修改', exact: true }).click()
        await expect(
            page.getByRole('heading', { name: `修改凭据“${providerLabel}”` }),
        ).toBeVisible()
        await expect(note).toHaveValue('')
        const remark = `${unique('e2e-note').padEnd(63, 'x')}🔑`
        await note.fill(`  ${remark}  `)
        await page.getByRole('button', { name: '取消', exact: true }).click()
        await expect(page.getByText('放弃修改')).toBeVisible()
        await page
            .getByRole('button', { name: '继续编辑', exact: true })
            .click()
        await page
            .getByLabel('当前密码', { exact: true })
            .fill(account.password)
        await page.getByRole('button', { name: '保存修改' }).click()
        await expect(page.getByText(`凭据“${remark}”已更新`)).toBeVisible()
        await expect(row.locator('.node-name strong')).toHaveText(remark)
        await expect(row.locator('.node-name small')).toHaveText(providerLabel)

        // The selected label is exactly the note, without an added provider suffix.
        await page.goto('/certificates')
        await page
            .getByRole('button', { name: '申请证书', exact: true })
            .click()
        await chooseCredential(page, credential.id, remark)
        const domain = `${unique('note')}.example.com`
        await page.getByLabel('域名', { exact: true }).fill(domain)
        await tick(page.getByLabel('自定义 ACME 目录'))
        await page.getByLabel('ACME 目录地址').fill(await closedDirectory())
        await page.getByRole('button', { name: '提交申请' }).click()
        const certRow = certificateRow(page, domain)
        await expect(certRow).toContainText(remark)
        await expect(certRow.getByTestId('certificate-status')).toHaveText(
            '签发失败',
            { timeout: 20_000 },
        )
        const certificateId = await certRow.getAttribute('data-certificate-id')

        // Clearing the old note preserves the write-only secrets and refreshes certificate metadata.
        await page.goto('/dns-providers')
        await row.getByRole('button', { name: '修改', exact: true }).click()
        await expect(note).toHaveValue(remark)
        await note.fill('')
        await page
            .getByLabel('当前密码', { exact: true })
            .fill(account.password)
        await page.getByRole('button', { name: '保存修改' }).click()
        await expect(
            page.getByText(`凭据“${providerLabel}”已更新`),
        ).toBeVisible()
        await expect(row.locator('.node-name strong')).toHaveText(providerLabel)
        await expect(row.locator('.secret-status')).not.toContainText('未设置')
        await page.reload()
        await row.getByRole('button', { name: '修改', exact: true }).click()
        await expect(note).toHaveValue('')
        await page.getByRole('button', { name: '取消', exact: true }).click()

        await page.goto('/certificates')
        await expect(certRow).toContainText(providerLabel)
        await expect(certRow).not.toContainText(remark)
        const response = await page.request.get(`${origin}/api/certificates`)
        expect(response.status()).toBe(200)
        const listed = (await response.json()).certificates as {
            id: string
            dnsCredentialName: string
            dnsCredentialProvider: string
        }[]
        expect(listed.find((item) => item.id === certificateId)).toMatchObject({
            dnsCredentialName: '',
            dnsCredentialProvider: provider,
        })
        await page
            .getByRole('button', { name: '申请证书', exact: true })
            .click()
        await chooseCredential(page, credential.id, providerLabel)
        await noHorizontalOverflow(page)
        await noStoredSecrets(page, account)

        const removed = await page.request.post(
            `${origin}/api/certificates/${certificateId}/delete`,
            { headers, data: {} },
        )
        expect(removed.status()).toBe(204)
        await page.goto('/dns-providers')
        await row.getByRole('button', { name: '删除', exact: true }).click()
        await page
            .getByLabel('当前密码', { exact: true })
            .fill(account.password)
        await page.getByRole('button', { name: '确认删除' }).click()
        await expect(
            page.getByText(`凭据“${providerLabel}”已删除`),
        ).toBeVisible()
        await expect(row).toHaveCount(0)
    })
}

test('credentials used by a certificate cannot be deleted', async ({
    page,
    account,
}) => {
    await signIn(page, account)
    const name = unique('e2e-used')
    const credentialId = await createCredential(page, account, name, secret())
    const certificateId = await requestCertificate(
        page,
        credentialId,
        `${unique('used')}.example.com`,
    )
    await page.goto('/dns-providers')
    const row = credentialRow(page, credentialId)
    await expect(row).toContainText('1 张证书使用')
    await row.getByRole('button', { name: '删除' }).click()
    await page.getByLabel('当前密码', { exact: true }).fill(account.password)
    await page.getByRole('button', { name: '确认删除' }).click()
    await expect(
        page.getByText('该凭据正被证书使用，请先删除相关证书。'),
    ).toBeVisible()
    await expect(row).toBeVisible()

    const removed = await page.request.post(
        `${origin}/api/certificates/${certificateId}/delete`,
        { headers, data: {} },
    )
    expect(removed.status()).toBe(204)
})

test('certificate requests validate input and show the real issuance state', async ({
    page,
    account,
}) => {
    await signIn(page, account)
    const credentialName = unique('e2e-cert')
    const credentialId = await createCredential(
        page,
        account,
        credentialName,
        secret(),
    )
    await page.goto('/certificates')
    await expect(
        page.getByRole('heading', { name: '证书', level: 1 }),
    ).toBeVisible()

    await page.getByRole('button', { name: '申请证书' }).click()
    const submit = page.getByRole('button', { name: '提交申请' })
    await submit.click()
    await expect(page.getByText('请至少输入一个域名。')).toBeVisible()
    await expect(page.getByLabel('域名', { exact: true })).toBeFocused()

    // The backend rejects names that are not DNS names.
    await page.getByLabel('域名', { exact: true }).fill('bad_name.example.com')
    await chooseCredential(page, credentialId, credentialName)
    await submit.click()
    await expect(page.getByText(/^域名格式不正确/)).toBeVisible()

    // Custom directories must use https://.
    const domain = `${unique('e2e')}.example.com`
    await page
        .getByLabel('域名', { exact: true })
        .fill(`${domain}\n*.${domain}`)
    await tick(page.getByLabel('自定义 ACME 目录'))
    await page.getByLabel('ACME 目录地址').fill('http://127.0.0.1/directory')
    await submit.click()
    await expect(
        page.getByText('请输入以 https:// 开头的 ACME 目录地址。'),
    ).toBeVisible()
    await page.getByLabel('EAB Key ID').fill('e2e-kid')
    await page.getByLabel('ACME 目录地址').fill(await closedDirectory())
    await submit.click()
    await expect(
        page.getByText('EAB Key ID 与 HMAC Key 需要同时填写。'),
    ).toBeVisible()
    const hmac = randomBytes(32).toString('base64url')
    await page.getByLabel('EAB HMAC Key').fill(hmac)
    await expect(page.getByLabel('EAB HMAC Key')).toHaveAttribute(
        'type',
        'password',
    )
    await noHorizontalOverflow(page)
    await submit.click()
    await expect(page.getByText('已提交申请，正在后台签发')).toBeVisible()

    // The real background job cannot reach the directory and fails.
    const row = certificateRow(page, domain)
    await expect(row).toContainText(`*.${domain}`)
    await expect(row).toContainText(credentialName)
    await expect(row.getByTestId('certificate-status')).toHaveText('签发失败', {
        timeout: 20_000,
    })
    await expect(row.getByTestId('certificate-error')).toContainText(
        '无法连接 ACME 服务，请检查目录地址与网络。',
    )
    await expect(row.getByRole('button', { name: '下载证书链' })).toHaveCount(0)
    await noStoredSecrets(page, account)
    const storage = await page.evaluate(() =>
        JSON.stringify({ ...localStorage, ...sessionStorage }),
    )
    expect(storage).not.toContain(hmac)
    const listed = await page.request.get(`${origin}/api/certificates`)
    expect(await listed.text()).not.toContain(hmac)

    // A chain that was never issued cannot be downloaded.
    const id = await row.getAttribute('data-certificate-id')
    const chain = await page.request.get(
        `${origin}/api/certificates/${id}/chain`,
    )
    expect(chain.status()).toBe(409)
    expect((await chain.json()).code).toBe('CERTIFICATE_NOT_ISSUED')

    // Reissue runs the job again and reaches the same real state.
    await row.getByRole('button', { name: '重新签发' }).click()
    await expect(page.getByText('已开始重新签发')).toBeVisible()
    await expect(row.getByTestId('certificate-status')).toHaveText('签发失败', {
        timeout: 20_000,
    })

    await row.getByRole('button', { name: '删除' }).click()
    await page.getByRole('button', { name: '确认删除' }).click()
    await expect(page.getByText('证书已删除')).toBeVisible()
    await expect(certificateRow(page, domain)).toHaveCount(0)
})

test('the console lists certificates and credentials behind the session', async ({
    page,
    account,
}) => {
    // Without a session the pages redirect to sign-in and the API refuses.
    await page.goto('/certificates')
    await expect(page).toHaveURL(/\/login\?redirect=(%2F|\/)certificates$/)
    const anonymous = await page.request.get(`${origin}/api/certificates`)
    expect(anonymous.status()).toBe(401)
    const write = await page.request.post(`${origin}/api/dns-credentials`, {
        data: { name: 'x', provider: 'cloudflare', secrets: {}, password: '' },
    })
    expect(write.status()).toBe(403)

    // Signing in returns to the protected page.
    await login(page, account)
    await expect(page).toHaveURL(/\/certificates$/)
    await expect(page.getByTestId('certificates-summary')).toHaveText(/张证书$/)
    await noHorizontalOverflow(page)
    await page.getByRole('link', { name: 'DNS 服务商', exact: true }).click()
    await expect(page).toHaveURL(/\/dns-providers$/)
    await expect(page.getByTestId('credentials-summary')).toHaveText(/个凭据$/)
    await noHorizontalOverflow(page)
})
