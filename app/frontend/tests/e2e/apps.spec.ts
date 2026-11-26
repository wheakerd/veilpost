import { randomUUID } from 'node:crypto'
import { mkdirSync, readFileSync, rmSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { expect, type Page, type Request } from '@playwright/test'
import {
    test,
    signIn,
    login,
    headers,
    noStoredSecrets,
    type Account,
} from './real-auth'
import { choose, chosen } from './element'

// Installation state is global in the shared e2e backend, and the desktop
// and mobile projects run these tests at the same time. Each test therefore
// owns one app for its whole run through a lock directory in the run's
// temporary data directory, starts by resetting that app through the real
// API, and asserts only that app. No other test file installs apps.
const origin = 'http://127.0.0.1:4173'
const root = resolve(import.meta.dirname, '../../../..')

async function withAppLock(app: string, run: () => Promise<void>) {
    const { dataDir } = JSON.parse(
        readFileSync(resolve(root, '.runtime/e2e/session.json'), 'utf8'),
    )
    const lock = join(dataDir, `e2e-app-lock-${app}`)
    const deadline = Date.now() + 60_000
    for (;;) {
        try {
            mkdirSync(lock)
            break
        } catch (error) {
            if ((error as NodeJS.ErrnoException).code !== 'EEXIST') throw error
            if (Date.now() > deadline)
                throw new Error(`app lock ${app} timed out`, { cause: error })
            await new Promise((done) => setTimeout(done, 100))
        }
    }
    try {
        await run()
    } finally {
        rmSync(lock, { recursive: true, force: true })
    }
}

async function installed(page: Page, app: string) {
    const response = await page.request.get(`${origin}/api/apps`)
    expect(response.status()).toBe(200)
    const { apps } = await response.json()
    return (
        apps.find((item: { id: string }) => item.id === app).installed !== null
    )
}

async function ensureUninstalled(page: Page, account: Account, app: string) {
    if (!(await installed(page, app))) return
    const response = await page.request.post(
        `${origin}/api/apps/${app}/uninstall`,
        { headers, data: { password: account.password } },
    )
    expect(response.status()).toBe(204)
}

async function ensureInstalled(page: Page, account: Account, app: string) {
    await ensureUninstalled(page, account, app)
    const response = await page.request.post(
        `${origin}/api/apps/${app}/install`,
        { headers, data: {} },
    )
    expect(response.status()).toBe(200)
}

function installedNavigation(page: Page) {
    return page.getByRole('navigation', { name: '已安装软件' })
}

async function expectNoSecret(page: Page, secret: string) {
    const storage = await page.evaluate(() =>
        JSON.stringify({
            ...localStorage,
            ...sessionStorage,
            c: document.cookie,
        }),
    )
    expect(storage).not.toContain(secret)
    expect(page.url()).not.toContain(secret)
    expect(await page.content()).not.toContain(secret)
}

test.beforeEach(async ({ page, account }) => {
    test.setTimeout(120_000)
    await signIn(page, account)
})

test('installing adds a sidebar entry and uninstalling with the password removes it', async ({
    page,
    account,
}, testInfo) => {
    await withAppLock('amneziawg', async () => {
        await ensureUninstalled(page, account, 'amneziawg')
        await page.goto('/')
        await expect(
            page.getByRole('heading', { name: '网络概览', level: 1 }),
        ).toBeVisible()
        // One store link on each layout: main navigation on desktop, the app
        // row below the brand on narrow screens (the bottom bar keeps 4 items).
        const storeLink = page.getByRole('link', {
            name: '软件商店',
            exact: true,
        })
        await expect(storeLink).toHaveCount(1)
        await storeLink.click()
        await expect(page).toHaveURL(`${origin}/store`)
        await expect(
            page.getByRole('heading', { name: '软件目录', exact: true }),
        ).toBeVisible()
        const item = page.locator('li[data-app-id="amneziawg"]')
        await expect(item).toContainText('AmneziaWG')
        await expect(item).toContainText('版本 3.1.20261116')
        await expect(item).toContainText('未安装')
        await expect(item).toContainText('UDP · 部署时按节点随机')
        await expect(item).toContainText(
            '由本仓库 docker/node/amneziawg 构建，尚无发布地址',
        )
        await expect(page.locator('li[data-app-id="xray"]')).toContainText(
            'ghcr.io/xtls/xray-core:26.11.9@sha256:9812e382e229e9ed63fd179bf1a4e3fe244c12ff3ae4fd8128745d79c87575ab',
        )
        await expect(
            installedNavigation(page).getByRole('link', { name: 'AmneziaWG' }),
        ).toHaveCount(0)

        await item.getByRole('button', { name: '安装', exact: true }).click()
        await expect(
            page.getByText('已安装“AmneziaWG”，侧栏中已出现它的页面。'),
        ).toBeVisible()
        await expect(item).toContainText('已安装')
        const entry = installedNavigation(page).getByRole('link', {
            name: 'AmneziaWG',
        })
        await expect(entry).toBeVisible()
        expect(
            await page.evaluate(
                () => document.documentElement.scrollWidth <= window.innerWidth,
            ),
        ).toBe(true)
        await page.screenshot({
            path: testInfo.outputPath('store.png'),
            fullPage: true,
            animations: 'disabled',
        })

        await entry.click()
        await expect(page).toHaveURL(`${origin}/apps/amneziawg`)
        await expect(
            page.getByRole('heading', { name: 'AmneziaWG', level: 1 }),
        ).toBeVisible()
        await expect(page.getByTestId('deploy-unavailable')).toContainText(
            '节点第 2 阶段',
        )
        await expect(
            page.getByRole('button', { name: '部署到节点' }),
        ).toBeDisabled()
        await expect(page.getByLabel('MTU')).toHaveValue('1280')
        // The page is a real History route that survives a refresh.
        await page.reload()
        await expect(
            page.getByRole('heading', { name: 'AmneziaWG', level: 1 }),
        ).toBeVisible()

        await page.getByRole('button', { name: '卸载', exact: true }).click()
        await page.getByLabel('当前密码').fill('not-the-current-password')
        await page.getByRole('button', { name: '确认卸载' }).click()
        await expect(
            page.getByText('当前密码不正确，请重新输入。'),
        ).toBeVisible()
        await expect(page.getByLabel('当前密码')).toHaveValue('')
        expect(await installed(page, 'amneziawg')).toBe(true)
        await page.getByLabel('当前密码').fill(account.password)
        await page.getByRole('button', { name: '确认卸载' }).click()
        await expect(
            page.getByRole('heading', { name: '“AmneziaWG”尚未安装' }),
        ).toBeVisible()
        await expect(
            page.getByText('“AmneziaWG”已卸载，配置已删除'),
        ).toBeVisible()
        await expect(entry).toHaveCount(0)
        expect(await installed(page, 'amneziawg')).toBe(false)
        await noStoredSecrets(page, account)
    })
})

test('an app that is not installed guides to the store, including deep links', async ({
    page,
    account,
}) => {
    await withAppLock('xray', async () => {
        await ensureUninstalled(page, account, 'xray')
        await page.goto('/apps/xray')
        await expect(
            page.getByRole('heading', { name: '“Xray-core”尚未安装' }),
        ).toBeVisible()
        await expect(
            installedNavigation(page).getByRole('link', { name: 'Xray-core' }),
        ).toHaveCount(0)
        const settings = await page.request.get(
            `${origin}/api/apps/xray/settings`,
        )
        expect(settings.status()).toBe(404)
        expect(await settings.json()).toEqual({ code: 'APP_NOT_INSTALLED' })

        const requests: string[] = []
        const record = (request: Request) => {
            requests.push(request.url())
        }
        page.on('request', record)
        await page
            .getByRole('navigation', { name: '主导航' })
            .getByRole('link', { name: 'VPN 节点', exact: true })
            .click()
        await expect(
            page.getByRole('heading', { name: 'VPN 节点', exact: true }),
        ).toBeVisible()
        page.off('request', record)
        expect(requests).not.toContain(`${origin}/api/apps`)
        await page.goBack()
        await expect(
            page.getByRole('heading', { name: '“Xray-core”尚未安装' }),
        ).toBeVisible()

        await page.getByRole('link', { name: '前往软件商店' }).click()
        await expect(page).toHaveURL(`${origin}/store`)
        await expect(
            page.getByRole('heading', { name: '软件商店', level: 1 }),
        ).toBeVisible()

        await page.goto('/apps/not-in-catalog')
        await expect(
            page.getByRole('heading', { name: '软件目录中没有这个软件' }),
        ).toBeVisible()

        // A signed-out deep link returns to the app page after signing in.
        await page.context().clearCookies()
        await page.goto('/apps/xray')
        await expect(page).toHaveURL(
            /\/login\?redirect=(%2F|\/)apps(%2F|\/)xray$/,
        )
        await login(page, account)
        await expect(page).toHaveURL(`${origin}/apps/xray`)
        await expect(
            page.getByRole('heading', { name: '“Xray-core”尚未安装' }),
        ).toBeVisible()
    })
})

test('the nodes page links to the store instead of listing protocols', async ({
    page,
}) => {
    await page.goto('/nodes')
    await expect(
        page.getByText('入口协议由软件商店中的 VPN 软件提供'),
    ).toBeVisible()
    await expect(page.getByText('计划接入的协议')).toHaveCount(0)
    await page.getByRole('link', { name: '前往软件商店' }).click()
    await expect(page).toHaveURL(`${origin}/store`)
})

test('settings are validated by the server and sensitive values are write-only', async ({
    page,
    account,
}, testInfo) => {
    const secret = `e2e-salamander-${randomUUID()}`
    await withAppLock('hysteria2', async () => {
        await ensureInstalled(page, account, 'hysteria2')
        await page.goto('/apps/hysteria2')
        await expect(page.getByTestId('settings-state')).toHaveText(
            '待配置：还有必填项未填写',
        )
        await expect(page.getByTestId('secret-salamanderPassword')).toHaveText(
            '未设置',
        )
        const port = page.getByLabel('UDP 端口')
        await expect(port).toHaveValue('443')

        await port.fill('0')
        await page.getByRole('button', { name: '保存配置' }).click()
        await expect(
            page.getByText('“UDP 端口”的值无效，请按说明修改后重试。'),
        ).toBeVisible()
        await expect(port).toBeFocused()
        await expect(port).toHaveAttribute('aria-invalid', 'true')

        await port.fill('8443')
        await choose(page, page.getByLabel('伪装方式'), 'Salamander 混淆')
        await page.getByRole('button', { name: '保存配置' }).click()
        await expect(
            page.getByText('请填写“Salamander 混淆密码”。'),
        ).toBeVisible()

        const password = page.getByLabel('Salamander 混淆密码')
        await password.fill(secret)
        await page.getByRole('button', { name: '保存配置' }).click()
        await expect(page.getByText('配置已保存')).toBeVisible()
        await expect(page.getByTestId('secret-salamanderPassword')).toHaveText(
            '已设置',
        )
        await expect(page.getByTestId('settings-state')).toHaveText('配置完整')
        await expect(password).toHaveValue('')
        await expect(password).toHaveAttribute(
            'placeholder',
            '留空则保留已保存的值',
        )
        await expectNoSecret(page, secret)
        for (const colorScheme of ['light', 'dark'] as const) {
            await page.emulateMedia({ colorScheme })
            await expect(page.locator('html')).toHaveCSS(
                'color-scheme',
                colorScheme,
            )
            await page.screenshot({
                path: testInfo.outputPath(`app-settings-${colorScheme}.png`),
                animations: 'disabled',
            })
        }

        // The server never returns the value, and it survives a reload as "set".
        for (const path of ['/api/apps', '/api/apps/hysteria2/settings']) {
            const response = await page.request.get(`${origin}${path}`)
            expect(response.status()).toBe(200)
            expect(await response.text()).not.toContain(secret)
        }
        await page.reload()
        await expect(page.getByLabel('UDP 端口')).toHaveValue('8443')
        await expect(chosen(page, page.getByLabel('伪装方式'))).toContainText(
            'Salamander 混淆',
        )
        await expect(page.getByTestId('secret-salamanderPassword')).toHaveText(
            '已设置',
        )
        await expectNoSecret(page, secret)

        // Clearing a required secret is rejected until the mode changes.
        await page.getByRole('button', { name: '清除已保存的值' }).click()
        await expect(page.getByText('保存后会清除已保存的值')).toBeVisible()
        await page.getByRole('button', { name: '保存配置' }).click()
        await expect(
            page.getByText('请填写“Salamander 混淆密码”。'),
        ).toBeVisible()
        await choose(page, page.getByLabel('伪装方式'), '伪装为 HTTP/3 站点')
        await page.getByLabel('伪装站点地址').fill('https://www.example.com/')
        await page.getByRole('button', { name: '保存配置' }).click()
        await expect(page.getByText('配置已保存')).toBeVisible()
        await expect(page.getByTestId('secret-salamanderPassword')).toHaveText(
            '未设置',
        )
        await expect(page.getByTestId('settings-state')).toHaveText('配置完整')
        await noStoredSecrets(page, account)
        await ensureUninstalled(page, account, 'hysteria2')
    })
})

test.describe('Russian interface', () => {
    test.use({ locale: 'ru' })

    test('the store fits the viewport with longer text', async ({ page }) => {
        await page.goto('/store')
        await expect(
            page.getByRole('heading', { name: 'Магазин', level: 1 }),
        ).toBeVisible()
        await expect(page.locator('li[data-app-id="xray"]')).toContainText(
            'Не установлено',
        )
        const overflow = await page.evaluate(() => {
            const issues: string[] = []
            if (document.documentElement.scrollWidth > window.innerWidth)
                issues.push('page')
            for (const element of document.querySelectorAll<HTMLElement>(
                'h1, h2, h3, .el-button, .el-tag, .nav-link, .app-manage-link',
            ))
                if (element.scrollWidth > element.clientWidth + 1)
                    issues.push(element.textContent.trim())
            return issues
        })
        expect(overflow).toEqual([])
    })
})
