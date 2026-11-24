import { execFileSync } from 'node:child_process'
import { createHash, createHmac, randomUUID } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { test as base, expect, type Page } from '@playwright/test'

export interface Account {
    username: string
    password: string
}
const root = resolve(import.meta.dirname, '../../../..')
const origin = 'http://127.0.0.1:4173'
export const headers = { Origin: origin, 'X-Requested-With': 'Veilpost' }

export const test = base.extend<{ account: Account }>({
    account: async ({}, use) => {
        const info = JSON.parse(
            readFileSync(resolve(root, '.runtime/e2e/session.json'), 'utf8'),
        )
        const account = {
            username: `e2e_${randomUUID().replaceAll('-', '').slice(0, 20)}`,
            password: `E2e-${randomUUID()}`,
        }
        execFileSync(
            resolve(root, 'app/backend/bin/veilpost-api'),
            [
                'admin-create',
                '--username',
                account.username,
                '--password-stdin',
            ],
            {
                env: {
                    ...process.env,
                    VPN_ENV: 'development',
                    VPN_DATA_DIR: info.dataDir,
                },
                input: `${account.password}\n`,
                stdio: ['pipe', 'pipe', 'pipe'],
            },
        )
        await use(account)
    },
})

// Signing out lives in the account centre, which opens from the top bar.
export async function signOut(page: Page) {
    await page.getByRole('button', { name: '打开个人中心' }).click()
    await page
        .getByRole('dialog', { name: '个人中心' })
        .getByRole('button', { name: '退出登录' })
        .click()
}

export async function signIn(page: Page, account: Account) {
    const response = await page.request.post(`${origin}/api/auth/login`, {
        headers,
        data: account,
    })
    expect(response.status()).toBe(200)
    expect((await response.json()).status).toBe('authenticated')
}

// Login form labels in each interface language (src/locales/*/auth.ts).
const loginLabels = {
    'zh-CN': { username: '账号', password: '密码', submit: '登录' },
    en: { username: 'Username', password: 'Password', submit: 'Sign in' },
    'zh-TW': { username: '帳號', password: '密碼', submit: '登入' },
    ru: { username: 'Имя пользователя', password: 'Пароль', submit: 'Войти' },
} as const

export async function login(
    page: Page,
    account: Account,
    password = account.password,
    language: keyof typeof loginLabels = 'zh-CN',
) {
    const labels = loginLabels[language]
    await page
        .getByLabel(labels.username, { exact: true })
        .fill(account.username)
    await page.getByLabel(labels.password, { exact: true }).fill(password)
    await page.getByRole('button', { name: labels.submit, exact: true }).click()
}

export function code(secret: string, offset = 0) {
    const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
    let bits = ''
    for (const letter of secret)
        bits += alphabet.indexOf(letter).toString(2).padStart(5, '0')
    const key = Buffer.from(
        bits.match(/.{8}/g)!.map((byte) => parseInt(byte, 2)),
    )
    const counter = Buffer.alloc(8)
    counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30_000) + offset))
    const hash = createHmac('sha1', key).update(counter).digest()
    const position = hash[19]! & 15
    return ((hash.readUInt32BE(position) & 0x7fffffff) % 1_000_000)
        .toString()
        .padStart(6, '0')
}

export async function enableMfa(page: Page, account: Account) {
    await signIn(page, account)
    const setupResponse = await page.request.post(
        `${origin}/api/auth/two-factor/setup`,
        { headers, data: { password: account.password } },
    )
    expect(setupResponse.status()).toBe(200)
    const setup = await setupResponse.json()
    const enabled = await page.request.post(
        `${origin}/api/auth/two-factor/enable`,
        { headers, data: { setupId: setup.setupId, code: code(setup.secret) } },
    )
    expect(enabled.status()).toBe(200)
    const { recoveryCodes } = await enabled.json()
    await page.request.post(`${origin}/api/auth/logout`, { headers, data: {} })
    return {
        secret: setup.secret as string,
        recoveryCodes: recoveryCodes as string[],
    }
}

export async function expireSession(page: Page) {
    const info = JSON.parse(
        readFileSync(resolve(root, '.runtime/e2e/session.json'), 'utf8'),
    )
    const cookie = (await page.context().cookies()).find(
        (item) => item.name === 'vpn_session',
    )!
    const tokenHash = createHash('sha256').update(cookie.value).digest('hex')
    execFileSync(
        'python3',
        [
            '-c',
            `import json,sqlite3,sys,pathlib,time
p=json.load(sys.stdin)
directory=pathlib.Path(p['directory']).resolve()
assert directory.parent == pathlib.Path(p['root']).resolve()
with sqlite3.connect(directory/'auth.sqlite') as db:
 db.execute('UPDATE sessions SET expires_at=? WHERE token_hash=?',(int(time.time()*1000)-1,p['hash']))
`,
        ],
        {
            input: JSON.stringify({
                directory: info.dataDir,
                root: resolve(root, '.runtime/e2e'),
                hash: tokenHash,
            }),
        },
    )
}

export async function noStoredSecrets(page: Page, account: Account) {
    const storage = await page.evaluate(() => ({
        local: { ...localStorage },
        session: { ...sessionStorage },
        cookie: document.cookie,
    }))
    expect(
        Object.keys(storage.local).filter(
            (key) => key !== 'veilpost:preferences:v1',
        ),
    ).toEqual([])
    expect(
        Object.keys(storage.session).filter(
            (key) => key !== 'vue-router:scroll',
        ),
    ).toEqual([])
    expect(JSON.stringify(storage)).not.toContain(account.password)
    expect(storage.cookie).not.toContain('vpn_session')
    const cookies = await page.context().cookies()
    expect(
        cookies.find((cookie) => cookie.name === 'vpn_session'),
    ).toMatchObject({ httpOnly: true, sameSite: 'Strict' })
}
