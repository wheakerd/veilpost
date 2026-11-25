import { createHash, generateKeyPairSync, randomBytes, sign } from 'node:crypto'
import { expect, type Locator, type Page } from '@playwright/test'
import {
    test,
    signIn,
    expireSession,
    headers,
    noStoredSecrets,
    type Account,
} from './real-auth'

// These tests drive the real Go backend. Enrollment and the signed poll are
// sent from the test with Node's Ed25519 implementation, which exercises the
// real /api/agent contract; they do not run the veilpost-agent program.
const origin = 'http://127.0.0.1:4173'

function base64url(data: Buffer) {
    return data.toString('base64url')
}

function nodeName() {
    return `e2e-node-${randomBytes(4).toString('hex')}`
}

async function createNode(page: Page, name: string, password: string) {
    await page.getByRole('button', { name: '创建节点' }).click()
    await page.getByLabel('节点名称').fill(name)
    await page.getByLabel('当前密码').fill(password)
    await page.getByRole('button', { name: '创建并生成注册令牌' }).click()
}

function row(page: Page, name: string): Locator {
    return page.getByRole('row').filter({ hasText: name })
}

async function enrollAgent(token: string) {
    const { publicKey, privateKey } = generateKeyPairSync('ed25519')
    const encoded = publicKey.export({ format: 'jwk' }).x!
    const proofMessage = [
        'vpn-agent-enroll-v1',
        createHash('sha256').update(token).digest('hex'),
        encoded,
    ].join('\n')
    const enrolled = await fetch(`${origin}/api/agent/enroll`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            token,
            publicKey: encoded,
            agentVersion: 'e2e-1.0',
            proof: base64url(sign(null, Buffer.from(proofMessage), privateKey)),
        }),
    })
    expect(enrolled.status).toBe(200)
    const { nodeId } = await enrolled.json()
    const poll = (fields: Record<string, unknown> = {}) => {
        const body = JSON.stringify({
            agentVersion: 'e2e-1.0',
            configVersion: 0,
            ...fields,
        })
        const timestamp = String(Math.floor(Date.now() / 1000))
        const nonce = base64url(randomBytes(24))
        const message = [
            'vpn-agent-v1',
            'POST',
            '/api/agent/poll',
            nodeId,
            timestamp,
            nonce,
            createHash('sha256').update(body).digest('hex'),
        ].join('\n')
        return fetch(`${origin}/api/agent/poll`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-VPN-Node': nodeId,
                'X-VPN-Timestamp': timestamp,
                'X-VPN-Nonce': nonce,
                'X-VPN-Signature': base64url(
                    sign(null, Buffer.from(message), privateKey),
                ),
            },
            body,
        })
    }
    return { nodeId: nodeId as string, poll }
}

async function enrollAndPoll(token: string) {
    const agent = await enrollAgent(token)
    // The console records the signed request at once, then holds the long poll.
    return { nodeId: agent.nodeId, poll: agent.poll() }
}

async function expectNoToken(page: Page, account: Account, token: string) {
    await noStoredSecrets(page, account)
    const storage = await page.evaluate(() =>
        JSON.stringify({ ...localStorage, ...sessionStorage }),
    )
    expect(storage).not.toContain(token)
    expect(page.url()).not.toContain(token)
}

test.beforeEach(async ({ page, account }) => {
    await signIn(page, account)
})

test('Docker installation uses the selected node source and shows the signed outcome', async ({
    page,
    account,
}) => {
    const name = nodeName()
    const created = await page.request.post(`${origin}/api/nodes`, {
        headers,
        data: { name, password: account.password },
    })
    expect(created.status()).toBe(201)
    const { enrollment } = await created.json()
    const agent = await enrollAgent(enrollment.token)
    const sources = ['official', 'mirror']
    const advertise = await agent.poll({
        configVersion: 1,
        docker: { sources },
    })
    expect(advertise.status).toBe(200)

    await page.goto('/nodes')
    const item = row(page, name)
    await item.getByRole('button', { name: '安装 Docker' }).click()
    await page.getByRole('combobox', { name: '软件源' }).press('ArrowDown')
    await page.getByRole('option', { name: 'mirror' }).click()
    await page.getByLabel('当前密码').fill(account.password)
    await page.getByRole('button', { name: '确认安装' }).click()
    await expect(item).toContainText('Docker · mirror · 等待节点')

    const claimed = await agent.poll({ docker: { sources } })
    expect(claimed.status).toBe(200)
    const { dockerInstall } = await claimed.json()
    expect(dockerInstall.source).toBe('mirror')
    const completed = await agent.poll({
        configVersion: 1,
        docker: {
            sources,
            result: { id: dockerInstall.id, status: 'succeeded' },
        },
    })
    expect(completed.status).toBe(200)
    await page.reload()
    await expect(item).toContainText('Docker · mirror · 已安装')
    await noStoredSecrets(page, account)
})

test('creating a node shows a one-time enrollment token that never persists', async ({
    page,
    account,
}, testInfo) => {
    const errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    const consoleText: string[] = []
    page.on('console', (message) => consoleText.push(message.text()))
    await page.goto('/nodes')
    await expect(page.getByRole('heading', { name: '节点列表' })).toBeVisible()
    const name = nodeName()

    await createNode(page, name, 'not-the-current-password')
    await expect(page.getByText('当前密码不正确，请重新输入。')).toBeVisible()
    await expect(page.getByLabel('当前密码')).toHaveValue('')
    await expect(page.getByLabel('节点名称')).toHaveValue(name)
    await page.getByLabel('当前密码').fill(account.password)
    await page.getByRole('button', { name: '创建并生成注册令牌' }).click()

    await expect(
        page.getByRole('heading', { name: `注册节点“${name}”` }),
    ).toBeFocused()
    await expect(page.getByTestId('enroll-command')).toHaveText(
        `veilpost-agent enroll --server ${origin} --token-stdin`,
    )
    const token = (await page.getByTestId('enroll-token').textContent())!.trim()
    expect(token).toMatch(/^[A-Za-z0-9_-]{43}$/)
    await page.screenshot({
        path: testInfo.outputPath('nodes-enrollment.png'),
        fullPage: true,
        animations: 'disabled',
    })
    await expect(row(page, name)).toContainText('等待注册')
    await expect(row(page, name)).toContainText('未知')
    expect(
        await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
    ).toBe(true)
    await expectNoToken(page, account, token)

    // The list API never returns the token again.
    const listed = await page.request.get(`${origin}/api/nodes`)
    expect(await listed.text()).not.toContain(token)

    await page.getByRole('button', { name: '完成，隐藏令牌' }).click()
    await expect(page.getByTestId('enroll-token')).toHaveCount(0)
    expect(await page.content()).not.toContain(token)

    // Reloading never brings the token back.
    await page.reload()
    await expect(row(page, name)).toContainText('等待注册')
    expect(await page.content()).not.toContain(token)
    await expectNoToken(page, account, token)
    expect(consoleText.join('\n')).not.toContain(token)
    expect(errors).toEqual([])
})

test('an enrolled node shows real online status and revocation needs the password', async ({
    page,
    account,
}, testInfo) => {
    const name = nodeName()
    const created = await page.request.post(`${origin}/api/nodes`, {
        headers,
        data: { name, password: account.password },
    })
    expect(created.status()).toBe(201)
    const { enrollment } = await created.json()
    const { nodeId, poll } = await enrollAndPoll(enrollment.token)

    await page.goto('/nodes')
    const item = row(page, name)
    await expect(item).toContainText('在线')
    await expect(item).toContainText('最后通信：刚刚')
    await expect(item).toContainText('e2e-1.0')
    await expect(page.getByTestId('nodes-summary')).toContainText('个在线')
    await page.screenshot({
        path: testInfo.outputPath('nodes-online.png'),
        fullPage: true,
        animations: 'disabled',
    })

    // A token that was already redeemed cannot enroll another key.
    const reused = await fetch(`${origin}/api/agent/enroll`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            token: enrollment.token,
            publicKey: base64url(randomBytes(32)),
            agentVersion: 'e2e-1.0',
            proof: base64url(randomBytes(64)),
        }),
    })
    expect(reused.status).toBeGreaterThanOrEqual(400)

    await item.getByRole('button', { name: '吊销', exact: true }).click()
    await page.getByLabel('当前密码').fill('not-the-current-password')
    await page.getByRole('button', { name: '确认吊销' }).click()
    await expect(page.getByText('当前密码不正确，请重新输入。')).toBeVisible()
    await expect(item).toContainText('在线')

    await page.getByLabel('当前密码').fill(account.password)
    await page.getByRole('button', { name: '确认吊销' }).click()
    await expect(page.getByText(`节点“${name}”已吊销`)).toBeVisible()
    await expect(item).toContainText('已吊销')
    await expect(
        item.getByRole('button', { name: '吊销', exact: true }),
    ).toHaveCount(0)

    // The waiting long poll is answered at once, and the node stays revoked.
    const answer = await poll
    expect(answer.status).toBe(403)
    expect(await answer.json()).toEqual({ code: 'NODE_REVOKED' })
    const listed = await page.request.get(`${origin}/api/nodes`)
    const node = (await listed.json()).nodes.find(
        (entry: { id: string }) => entry.id === nodeId,
    )
    expect(node).toMatchObject({ status: 'revoked', online: false })
    await page.reload()
    await expect(row(page, name)).toContainText('已吊销')
})

test('node writes require the same-origin CSRF rules and a full session', async ({
    page,
    account,
}) => {
    const withoutHeader = await page.request.post(`${origin}/api/nodes`, {
        headers: { Origin: origin },
        data: { name: nodeName(), password: account.password },
    })
    expect(withoutHeader.status()).toBe(403)
    const crossSite = await page.request.post(`${origin}/api/nodes`, {
        headers: { ...headers, Origin: 'http://evil.example' },
        data: { name: nodeName(), password: account.password },
    })
    expect(crossSite.status()).toBe(403)

    await page.goto('/nodes')
    await expect(page.getByRole('heading', { name: '节点列表' })).toBeVisible()
    const name = nodeName()
    await page.getByRole('button', { name: '创建节点' }).click()
    await page.getByLabel('节点名称').fill(name)
    await page.getByLabel('当前密码').fill(account.password)
    await expireSession(page)
    await page.getByRole('button', { name: '创建并生成注册令牌' }).click()
    await expect(
        page.getByRole('heading', { name: '登录控制台' }),
    ).toBeVisible()
    await expect(page).toHaveURL(/\/login\?.*reason=expired/)
    expect(new URL(page.url()).searchParams.get('redirect')).toBe('/nodes')
    await expect(page.getByLabel('密码', { exact: true })).toHaveValue('')
    const anonymous = await page.request.get(`${origin}/api/nodes`)
    expect(anonymous.status()).toBe(401)
})

test('pending nodes can get a new token and revoked nodes can be deleted', async ({
    page,
    account,
}) => {
    await page.goto('/nodes')
    await expect(page.getByRole('heading', { name: '节点列表' })).toBeVisible()
    const name = nodeName()
    await createNode(page, name, account.password)
    const first = (await page.getByTestId('enroll-token').textContent())!.trim()
    await page.getByRole('button', { name: '完成，隐藏令牌' }).click()
    const item = row(page, name)
    await expect(item).toContainText('等待注册')

    await item.getByRole('button', { name: '重新签发令牌' }).click()
    await page.getByLabel('当前密码').fill(account.password)
    await page.getByRole('button', { name: '签发新令牌' }).click()
    await expect(
        page.getByRole('heading', { name: `注册节点“${name}”` }),
    ).toBeFocused()
    const second = (await page
        .getByTestId('enroll-token')
        .textContent())!.trim()
    expect(second).toMatch(/^[A-Za-z0-9_-]{43}$/)
    expect(second).not.toBe(first)
    await expectNoToken(page, account, second)
    await page.getByRole('button', { name: '完成，隐藏令牌' }).click()

    // The replaced token no longer enrolls; the new one still would.
    const stale = await fetch(`${origin}/api/agent/enroll`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            token: first,
            publicKey: base64url(randomBytes(32)),
            agentVersion: 'e2e-1.0',
            proof: base64url(randomBytes(64)),
        }),
    })
    expect(stale.status).toBeGreaterThanOrEqual(400)

    // Only revoked nodes offer deletion.
    await expect(item.getByRole('button', { name: '删除' })).toHaveCount(0)
    await item.getByRole('button', { name: '吊销', exact: true }).click()
    await page.getByLabel('当前密码').fill(account.password)
    await page.getByRole('button', { name: '确认吊销' }).click()
    await expect(item).toContainText('已吊销')
    await expect(
        item.getByRole('button', { name: '重新签发令牌' }),
    ).toHaveCount(0)

    await item.getByRole('button', { name: '删除' }).click()
    await page.getByLabel('当前密码').fill('not-the-current-password')
    await page.getByRole('button', { name: '确认删除' }).click()
    await expect(page.getByText('当前密码不正确，请重新输入。')).toBeVisible()
    await expect(item).toContainText('已吊销')
    await page.getByLabel('当前密码').fill(account.password)
    await page.getByRole('button', { name: '确认删除' }).click()
    await expect(page.getByText(`节点“${name}”已删除`)).toBeVisible()
    await expect(row(page, name)).toHaveCount(0)
    await page.reload()
    await expect(page.getByRole('heading', { name: '节点列表' })).toBeVisible()
    await expect(row(page, name)).toHaveCount(0)
})

test('the overview reports real node status instead of fixed text', async ({
    page,
}) => {
    await page.goto('/')
    await expect(page.getByTestId('overview-nodes')).toHaveText(
        /^(尚未接入|\d+ \/ \d+ 在线)$/,
    )
})
