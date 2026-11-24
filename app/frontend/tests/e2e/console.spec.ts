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
    await expect(page.locator('html')).toHaveAttribute(
        'data-reduced-motion',
        'true',
    )
    await page.emulateMedia({
        colorScheme: 'light',
        reducedMotion: 'no-preference',
    })
    await expect(page.locator('html')).not.toHaveClass('dark')
    await expect(page.locator('html')).toHaveAttribute(
        'data-reduced-motion',
        'false',
    )
})

test('all pages fit the viewport and both color modes render', async ({
    page,
}, testInfo) => {
    // Sixteen full-page captures of every palette in both modes.
    test.slow()
    for (const route of [
        '/',
        '/nodes',
        '/settings/security',
        '/settings/appearance',
    ]) {
        await page.goto(route)
        const titles: Record<string, string> = {
            '/': '网络概览',
            '/nodes': 'VPN 节点',
            '/settings/security': '账号安全',
            '/settings/appearance': '偏好设置',
        }
        await expect(
            page.getByRole('heading', { name: titles[route], exact: true }),
        ).toBeVisible()
        expect(
            await page.evaluate(
                () => document.documentElement.scrollWidth <= window.innerWidth,
            ),
        ).toBe(true)
    }
    for (const name of [
        '深海蓝',
        '晴空蓝',
        '翡翠绿',
        '雾海青',
        '琥珀橙',
        '珊瑚橙',
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
            await page.screenshot({
                path: testInfo.outputPath(`${name}-${mode}.png`),
                fullPage: true,
                animations: 'disabled',
            })
        }
    }
    await page.goto('/')
    await page.screenshot({
        path: testInfo.outputPath('overview.png'),
        fullPage: true,
        animations: 'disabled',
    })
})
