import { expect } from '@playwright/test'
import { test, signIn } from './real-auth'
import { paletteOption, tick } from './element'

test.beforeEach(async ({ page, account }) => {
    await signIn(page, account)
})

test('History routes load directly, refresh, navigate and handle missing pages', async ({
    page,
}) => {
    const errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    await page.goto('/nodes')
    await expect(
        page.getByRole('heading', { name: 'VPN 节点', exact: true }),
    ).toBeVisible()
    await page.reload()
    // Node tests share the backend, so the list may hold their nodes.
    await expect(page.getByRole('heading', { name: '节点列表' })).toBeVisible()
    await expect(page.getByTestId('nodes-summary')).not.toHaveText('正在读取')
    await page
        .getByRole('navigation', { name: '主导航' })
        .getByRole('link', { name: '网络概览', exact: true })
        .click()
    await expect(page).toHaveURL('http://127.0.0.1:4173/')
    await expect(page.getByRole('heading', { name: '节点状态' })).toBeVisible()
    await page.getByRole('button', { name: '查看 VPN 节点' }).click()
    await expect(page).toHaveURL('http://127.0.0.1:4173/nodes')
    await page.goBack()
    await expect(page.getByRole('heading', { name: '节点状态' })).toBeVisible()
    await page.goto('/not-a-real-route')
    await expect(
        page.getByRole('heading', { name: '这个页面不存在' }),
    ).toBeVisible()
    await page.getByRole('button', { name: '返回网络概览' }).click()
    await expect(page).toHaveURL('http://127.0.0.1:4173/')
    expect(errors).toEqual([])
})

test('palette, appearance and reduced motion survive reload and can reset', async ({
    page,
}) => {
    await page.emulateMedia({
        colorScheme: 'light',
        reducedMotion: 'no-preference',
    })
    await page.goto('/settings/appearance')
    await tick(paletteOption(page, '玫瑰红'))
    await page
        .getByRole('radiogroup', { name: '界面外观' })
        .getByText('深色', { exact: true })
        .click()
    await page.getByRole('switch', { name: '减少动态效果' }).press('Space')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'rose')
    await expect(page.locator('html')).toHaveClass('dark')
    await page.reload()
    await expect(paletteOption(page, '玫瑰红')).toBeChecked()
    await expect(
        page.getByRole('radio', { name: '深色', exact: true }),
    ).toBeChecked()
    await expect(
        page.getByRole('switch', { name: '减少动态效果' }),
    ).toBeChecked()
    await expect(page.locator('html')).toHaveAttribute(
        'data-reduced-motion',
        'true',
    )
    const saved = await page.evaluate(() =>
        JSON.parse(localStorage.getItem('veilpost:preferences:v1')!),
    )
    expect(saved).toEqual({
        palette: 'rose',
        mode: 'dark',
        reducedMotion: true,
        vision: 'standard',
        glassTint: 20,
        glassStrength: 60,
        pageTransition: 'fade',
        navMultiple: true,
        tabBar: true,
        progressBar: true,
        pageCache: true,
    })
    await page.getByRole('button', { name: '全部恢复默认' }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'abyss')
    await expect(page.locator('html')).not.toHaveClass('dark')
    await page.reload()
    await expect(page.getByRole('radio', { name: '跟随系统' })).toBeChecked()
})

test('system preferences react to changes and invalid stored values recover', async ({
    page,
}) => {
    await page.addInitScript(() => {
        localStorage.setItem(
            'veilpost:preferences:v1',
            JSON.stringify({
                palette: 'invalid',
                mode: 'invalid',
                reducedMotion: 'yes',
            }),
        )
    })
    await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' })
    await page.goto('/')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'abyss')
    await expect(page.locator('html')).toHaveClass('dark')
    const mode = page.getByRole('button', { name: /^界面外观：/ })
    await expect(mode).toHaveAccessibleName('界面外观：深色')
    await expect(page.locator('html')).toHaveAttribute(
        'data-reduced-motion',
        'true',
    )
    await page.emulateMedia({
        colorScheme: 'light',
        reducedMotion: 'no-preference',
    })
    await expect(page.locator('html')).not.toHaveClass('dark')
    await expect(mode).toHaveAccessibleName('界面外观：浅色')
    await expect(page.locator('html')).toHaveAttribute(
        'data-reduced-motion',
        'false',
    )
})

test('all pages fit the viewport and both color modes render', async ({
    page,
}, testInfo) => {
    test.slow()
    if (testInfo.project.name === 'desktop') {
        await page.setViewportSize({ width: 1920, height: 900 })
    }
    const routes = [
        ['overview', '/', '网络概览'],
        ['nodes', '/nodes', 'VPN 节点'],
        ['store', '/store', '软件商店'],
        ['app-guidance', '/apps/not-in-catalog', '软件目录中没有这个软件'],
        ['certificates', '/certificates', '证书'],
        ['dns', '/dns-providers', 'DNS 服务商'],
        ['security', '/settings/security', '账号安全'],
        ['not-found', '/not-a-real-route', '这个页面不存在'],
        ['appearance', '/settings/appearance', '偏好设置'],
    ] as const
    for (const [name, path, title] of routes) {
        await page.goto(path)
        await expect(
            page.getByRole('heading', { level: 1, name: title, exact: true }),
        ).toBeVisible()
        await expect(page.locator('#main-content')).toHaveAttribute(
            'aria-busy',
            'false',
        )
        if (path === '/') {
            const links = page.locator('.stat-row').getByRole('link')
            await expect(links).toHaveCount(4)
            expect(
                await links.evaluateAll((items) =>
                    items.map((item) => item.getAttribute('href')),
                ),
            ).toEqual([
                '/nodes',
                '/store',
                '/certificates',
                '/settings/security',
            ])
        }
        for (const colorScheme of ['light', 'dark'] as const) {
            await page.emulateMedia({ colorScheme })
            await expect(page.locator('html')).toHaveCSS(
                'color-scheme',
                colorScheme,
            )
            expect(
                await page.evaluate(
                    () => document.documentElement.scrollWidth <= innerWidth,
                ),
            ).toBe(true)
            await page.screenshot({
                path: testInfo.outputPath(`${name}-${colorScheme}.png`),
                animations: 'disabled',
            })
        }
    }
    for (const name of [
        '实验室绿',
        '深海蓝',
        '晴空蓝',
        '琥珀橙',
        '玫瑰红',
        '电光紫',
    ]) {
        await tick(paletteOption(page, name))
        for (const mode of ['浅色', '深色']) {
            await page
                .getByRole('radiogroup', { name: '界面外观' })
                .getByText(mode, { exact: true })
                .click()
            await expect(
                page.getByRole('radio', { name: mode, exact: true }),
            ).toBeChecked()
        }
    }
})
