import { expect, type Locator, type Page } from '@playwright/test'
import { test, signIn, noStoredSecrets } from './real-auth'
import { labelOf, paletteOption, tick } from './element'

const preferencesKey = 'veilpost:preferences:v1'

async function stored(page: Page) {
    return JSON.parse(
        (await page.evaluate(
            (key) => localStorage.getItem(key),
            preferencesKey,
        )) ?? '{}',
    ) as Record<string, unknown>
}

test.beforeEach(async ({ page, account }) => {
    await signIn(page, account)
    await page.emulateMedia({ colorScheme: 'light' })
})

test('global theme variables reach page content and Element Plus controls', async ({
    page,
}) => {
    await page.goto('/settings/appearance')
    const chip = labelOf(paletteOption(page, '深海蓝'))
    await expect(chip).toBeVisible()
    await page.locator('html').evaluate((root) => {
        root.style.setProperty('--app-primary', 'rgb(35, 83, 165)')
        root.style.setProperty('--app-font-size-small', '19px')
        root.style.setProperty('--app-font-size-body', '18px')
        root.style.setProperty('--app-space-8', '11px')
        root.style.setProperty('--app-radius-card', '13px')
        root.style.setProperty('--app-border-width', '3px')
    })
    await expect(chip).toHaveCSS('font-size', '19px')
    await expect(chip).toHaveCSS('border-top-left-radius', '13px')
    await expect(chip).toHaveCSS('border-top-width', '3px')
    await expect(chip).toHaveCSS('padding-inline-start', '11px')
    await expect(chip.locator('.palette-indicator')).toHaveCSS(
        'background-color',
        'rgb(35, 83, 165)',
    )
    const modes = page.getByRole('radiogroup', { name: '界面外观' })
    await expect(modes).toHaveCSS('font-size', '18px')
    await expect(modes).toHaveCSS('border-top-width', '3px')
    await expect(modes).toHaveCSS('--el-color-primary', 'rgb(35, 83, 165)')
})

test('the toolbar opens the preferences drawer and changes apply at once', async ({
    page,
    account,
}) => {
    await page.goto('/nodes')
    await page.getByRole('button', { name: '偏好设置' }).click()
    const drawer = page.getByRole('dialog', { name: '偏好设置' })
    await expect(drawer).toBeVisible()
    const html = page.locator('html')

    await drawer
        .getByRole('radiogroup', { name: '视觉模式' })
        .getByText('色弱友好', { exact: true })
        .click()
    await expect(html).toHaveAttribute('data-vision', 'colorSafe')
    await drawer
        .getByRole('radiogroup', { name: '视觉模式' })
        .getByText('灰阶模式', { exact: true })
        .click()
    await expect(html).toHaveCSS('filter', 'grayscale(1)')

    // Glass strength moves by keyboard and resets together with the tint.
    const before = await html.evaluate((root) =>
        root.style.getPropertyValue('--glass-blur'),
    )
    await drawer.getByRole('slider', { name: '强度' }).press('End')
    await expect
        .poll(() =>
            html.evaluate((root) =>
                root.style.getPropertyValue('--glass-blur'),
            ),
        )
        .not.toBe(before)
    expect(await stored(page)).toMatchObject({
        vision: 'grayscale',
        glassStrength: 100,
    })
    await drawer
        .getByRole('button', { name: '恢复默认', exact: true })
        .first()
        .click()
    expect(await stored(page)).toMatchObject({
        glassTint: 20,
        glassStrength: 60,
    })

    await tick(drawer.getByRole('radio', { name: '缩放聚焦' }))
    expect(await stored(page)).toMatchObject({ pageTransition: 'zoom' })

    await page.keyboard.press('Escape')
    await expect(drawer).toBeHidden()
    await expect(
        page.getByRole('heading', { name: 'VPN 节点', level: 1 }),
    ).toBeVisible()
    await noStoredSecrets(page, account)
})

// The sheet's vertical offset from its resting place, in pixels.
function sheetOffset(page: Page) {
    return page.getByRole('dialog', { name: '偏好设置' }).evaluate((sheet) => {
        const transform = getComputedStyle(sheet).transform
        return Math.round(
            transform === 'none' ? 0 : new DOMMatrixReadOnly(transform).m42,
        )
    })
}

test('on narrow screens preferences are a sheet that pulls down to close', async ({
    page,
}, testInfo) => {
    test.skip(
        testInfo.project.name !== 'mobile',
        'The bottom sheet is the narrow-screen drawer',
    )
    await page.goto('/')
    const open = page.getByRole('button', { name: '偏好设置' })
    const sheet = page.getByRole('dialog', { name: '偏好设置' })
    await open.click()
    await expect(sheet).toBeVisible()
    await expect.poll(() => sheetOffset(page)).toBe(0)
    const box = await sheet
        .getByRole('heading', { name: '偏好设置' })
        .boundingBox()
    if (!box) throw new Error('The sheet heading has no box')
    const x = box.x + box.width / 2
    const y = box.y + box.height / 2

    // A slow, short pull follows the pointer and settles back on release.
    await page.mouse.move(x, y)
    await page.mouse.down()
    for (let step = 1; step <= 10; step++) {
        await page.mouse.move(x, y + step * 8)
        await page.waitForTimeout(20)
    }
    expect(await sheetOffset(page)).toBeGreaterThan(60)
    await page.mouse.up()
    await expect.poll(() => sheetOffset(page)).toBe(0)
    await expect(sheet).toBeVisible()

    // Keep the flick's 20 ms sample intervals independent of runner latency.
    const session = await page.context().newCDPSession(page)
    const steps = 6
    const distance = 240
    const sampleInterval = 0.02
    const startedAt = Date.now() / 1000 - (steps + 1) * sampleInterval
    await session.send('Input.dispatchMouseEvent', {
        type: 'mousePressed',
        x,
        y,
        button: 'left',
        buttons: 1,
        clickCount: 1,
        timestamp: startedAt,
    })
    for (let step = 1; step <= steps; step++) {
        const offset = (step * distance) / steps
        await session.send('Input.dispatchMouseEvent', {
            type: 'mouseMoved',
            x,
            y: y + offset,
            button: 'left',
            buttons: 1,
            timestamp: startedAt + step * sampleInterval,
        })
        await expect.poll(() => sheetOffset(page)).toBe(offset)
    }
    await session.send('Input.dispatchMouseEvent', {
        type: 'mouseReleased',
        x,
        y: y + distance,
        button: 'left',
        buttons: 0,
        clickCount: 1,
        timestamp: startedAt + (steps + 1) * sampleInterval,
    })
    await session.detach()
    await expect(sheet).toBeHidden()

    // It opens again at rest, not where the flick left it.
    await open.click()
    await expect(sheet).toBeVisible()
    await expect.poll(() => sheetOffset(page)).toBe(0)
})

test('the tab bar and loading bar can be turned off', async ({
    page,
}, testInfo) => {
    test.skip(
        testInfo.project.name === 'mobile',
        'the tab strip is desktop only',
    )
    await page.goto('/settings/appearance')
    const tabs = page.getByRole('navigation', { name: '已打开的页面' })
    await expect(tabs).toBeVisible()
    await page.getByRole('switch', { name: '启用页签栏' }).press('Space')
    await expect(tabs).toBeHidden()
    await page.getByRole('switch', { name: '加载进度条' }).press('Space')
    expect(await stored(page)).toMatchObject({
        tabBar: false,
        progressBar: false,
    })
    await page.reload()
    await expect(
        page.getByRole('heading', { name: '偏好设置', level: 1 }),
    ).toBeVisible()
    await expect(tabs).toBeHidden()
    await page.getByRole('switch', { name: '启用页签栏' }).press('Space')
    await expect(tabs).toBeVisible()
})

test('sidebar groups fold, and with one group at a time opening one folds the others', async ({
    page,
}, testInfo) => {
    test.skip(
        testInfo.project.name === 'mobile',
        'narrow screens use the bottom bar',
    )
    await page.goto('/settings/appearance')
    const main = page.getByRole('navigation', { name: '主导航' })
    // Groups are Element Plus submenus; their titles fold and unfold them.
    const group = (name: string) =>
        main.locator('.el-sub-menu').filter({
            has: page.locator('.el-sub-menu__title', { hasText: name }),
        })
    const title = (name: string) =>
        group(name).locator('.el-sub-menu__title').first()
    await expect(group('安全')).toHaveAttribute('aria-expanded', 'true')
    await title('安全').click()
    await expect(group('安全')).toHaveAttribute('aria-expanded', 'false')
    await expect(main.getByRole('link', { name: '证书' })).toBeHidden()
    await title('安全').click()
    await expect(main.getByRole('link', { name: '证书' })).toBeVisible()

    await page.getByRole('switch', { name: '菜单栏行为' }).press('Space')
    // Single-group mode folds every group on the preferences page.
    await expect(group('工作空间')).toHaveAttribute('aria-expanded', 'false')
    await expect(group('软件')).toHaveAttribute('aria-expanded', 'false')
    await expect(group('安全')).toHaveAttribute('aria-expanded', 'false')

    await title('安全').click()
    await expect(group('安全')).toHaveAttribute('aria-expanded', 'true')
    await title('工作空间').click()
    await expect(group('工作空间')).toHaveAttribute('aria-expanded', 'true')
    await expect(group('安全')).toHaveAttribute('aria-expanded', 'false')
    // Navigating opens the destination's group.
    await main.getByRole('link', { name: 'VPN 节点' }).click()
    await expect(page).toHaveURL(/\/nodes$/)
    expect(await stored(page)).toMatchObject({ navMultiple: false })
})

test('the sidebar edge resizes by pointer and keyboard and restores its default', async ({
    page,
}, testInfo) => {
    test.skip(
        testInfo.project.name === 'mobile',
        'narrow screens use the bottom bar',
    )
    await page.goto('/settings/appearance')
    const sidebar = page.locator('.sidebar')
    const handle = page.getByRole('separator', { name: '调整侧栏宽度' })
    const start = (await sidebar.boundingBox())!.width

    const box = (await handle.boundingBox())!
    await page.mouse.move(box.x + box.width / 2, box.y + 200)
    await page.mouse.down()
    await page.mouse.move(box.x + box.width / 2 + 48, box.y + 200, {
        steps: 6,
    })
    // The edge follows the pointer while it is held.
    await expect
        .poll(async () => (await sidebar.boundingBox())!.width)
        .toBeGreaterThan(start + 40)
    await page.mouse.up()
    const dragged = (await stored(page)).sidebarWidth
    expect(typeof dragged).toBe('number')

    // Past the maximum it resists, then settles at the limit.
    await handle.focus()
    await handle.press('End')
    await expect.poll(async () => (await stored(page)).sidebarWidth).toBe(22)
    await expect(
        page.getByText('已固定为 22 rem', { exact: false }),
    ).toBeVisible()

    await page
        .getByRole('button', { name: '恢复默认', exact: true })
        .last()
        .click()
    expect(await stored(page)).not.toHaveProperty('sidebarWidth')
    await expect
        .poll(async () => Math.round((await sidebar.boundingBox())!.width))
        .toBe(Math.round(start))
})

test('kept pages return with their state while secrets are cleared', async ({
    page,
}, testInfo) => {
    await page.goto('/nodes')
    await page.getByRole('button', { name: '创建节点' }).click()
    await page.getByLabel('节点名称').fill('kept-name')
    await page.getByLabel('当前密码', { exact: true }).fill('not-the-password')
    const main = page.getByRole('navigation', { name: '主导航' })
    await main.getByRole('link', { name: '网络概览', exact: true }).click()
    await expect(
        page.getByRole('heading', { name: '网络概览', exact: true }),
    ).toBeVisible()
    await main.getByRole('link', { name: 'VPN 节点', exact: true }).click()
    await expect(page.getByLabel('节点名称')).toHaveValue('kept-name')
    await expect(page.getByLabel('当前密码', { exact: true })).toHaveValue('')

    // The refresh button reloads the page from scratch.
    if (testInfo.project.name !== 'mobile') {
        await page.getByRole('button', { name: '刷新当前页' }).click()
        await expect(page.getByLabel('节点名称')).toBeHidden()
    }

    // Without the page cache every visit starts fresh.
    await page.goto('/settings/appearance')
    await page.getByRole('switch', { name: '页面状态缓存' }).press('Space')
    await main.getByRole('link', { name: 'VPN 节点', exact: true }).click()
    await page.getByRole('button', { name: '创建节点' }).click()
    await page.getByLabel('节点名称').fill('fresh-name')
    await main.getByRole('link', { name: '网络概览', exact: true }).click()
    await main.getByRole('link', { name: 'VPN 节点', exact: true }).click()
    await expect(page.getByLabel('节点名称')).toBeHidden()
    expect(await stored(page)).toMatchObject({ pageCache: false })
})

test('page transition previews play only on hover', async ({ page }) => {
    await page.goto('/settings/appearance')
    const option = page.getByRole('radio', { name: '侧向滑入' })
    const preview = labelOf(option).locator('.preview-page')
    await expect(preview).toHaveCSS('animation-name', 'none')
    await labelOf(option).hover()
    await expect(preview).toHaveCSS('animation-name', 'preview-slide')
})

test('preferences saved under the old key carry over once', async ({
    page,
}) => {
    await page.addInitScript(() => {
        // Only on the first load: a reload must not bring the old key back.
        if (sessionStorage.getItem('seeded')) return
        sessionStorage.setItem('seeded', '1')
        localStorage.setItem(
            'vpn-console:preferences:v1',
            JSON.stringify({ palette: 'coral', mode: 'dark' }),
        )
    })
    await page.goto('/')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'rose')
    await expect(page.locator('html')).toHaveClass(/dark/)
    const keys = await page.evaluate(() => Object.keys(localStorage))
    expect(keys).toEqual([preferencesKey])
    expect(await stored(page)).toMatchObject({
        palette: 'coral',
        mode: 'dark',
    })
    await page.goto('/settings/appearance')
    await expect(paletteOption(page, '玫瑰红')).toBeChecked()
    await page
        .getByRole('radiogroup', { name: '界面外观' })
        .getByText('浅色', { exact: true })
        .click()
    await expect
        .poll(() => stored(page))
        .toMatchObject({
            palette: 'rose',
            mode: 'light',
        })
    await page.evaluate(() => {
        sessionStorage.clear()
    })
})

// Measure the cards once the drawer has settled.
async function expectPaletteColumns(group: Locator, columns: number) {
    const chips = group.locator('label')
    await expect(chips).toHaveCount(6)
    await expect
        .poll(() =>
            chips.evaluateAll(
                (elements) =>
                    new Set(
                        elements.map((element) =>
                            Math.round(element.getBoundingClientRect().x),
                        ),
                    ).size,
            ),
        )
        .toBe(columns)
    expect(
        await chips.evaluateAll((elements) =>
            elements.every(
                (element) => element.scrollWidth <= element.clientWidth,
            ),
        ),
    ).toBe(true)
}

async function previewColors(scope: Locator) {
    return scope
        .locator('.palette-preview')
        .evaluateAll((previews) =>
            previews.map((preview) =>
                Array.from(
                    preview.children,
                    (bar) => getComputedStyle(bar).backgroundColor,
                ),
            ),
        )
}

async function previewTokens(card: Locator, mode: 'light' | 'dark') {
    return card.evaluate((element, scheme) => {
        const style = getComputedStyle(element)
        return ['primary', 'secondary'].map((role) =>
            style.getPropertyValue(`--preview-${scheme}-${role}`).trim(),
        )
    }, mode)
}

test('palette cards preview both modes and apply the chosen colors', async ({
    page,
}, testInfo) => {
    // A palette retired from the list falls back to the default.
    await page.addInitScript((key) => {
        if (sessionStorage.getItem('seeded')) return
        sessionStorage.setItem('seeded', '1')
        localStorage.setItem(key, JSON.stringify({ palette: 'harbor' }))
    }, preferencesKey)
    await page.goto('/nodes')
    const html = page.locator('html')
    await expect(html).toHaveAttribute('data-theme', 'abyss')
    const activeColors = () =>
        html.evaluate((root) => {
            const style = getComputedStyle(root)
            return ['--app-primary', '--app-secondary'].map((token) =>
                style.getPropertyValue(token).trim(),
            )
        })
    const createNode = page.getByRole('button', {
        name: '创建节点',
        exact: true,
    })
    const buttonColors = () =>
        createNode.evaluate((element) => {
            const { backgroundColor, color } = getComputedStyle(element)
            return { backgroundColor, color }
        })
    const defaultLight = await buttonColors()

    await page.getByRole('button', { name: '偏好设置' }).click()
    const drawer = page.getByRole('dialog', { name: '偏好设置' })
    await expect(drawer).toBeVisible()
    const group = drawer.getByRole('radiogroup', { name: '主题色' })
    await expectPaletteColumns(group, 2)
    const samples = await previewColors(group)
    expect(samples).toHaveLength(12)
    for (const sample of samples) {
        expect(sample).toHaveLength(2)
        expect(new Set(sample).size).toBe(2)
        expect(sample).not.toContain('rgba(0, 0, 0, 0)')
    }
    const chosen = group.getByRole('radio', { name: '深海蓝', exact: true })
    await expect(chosen).toBeChecked()
    await expect(labelOf(chosen).locator('svg')).toBeVisible()
    await expect(
        labelOf(
            group.getByRole('radio', { name: '晴空蓝', exact: true }),
        ).locator('svg'),
    ).toHaveCount(0)
    await expect(labelOf(chosen).locator('.palette-indicator')).toHaveCSS(
        'color',
        'rgb(11, 13, 17)',
    )
    const amber = group.getByRole('radio', { name: '琥珀橙', exact: true })
    await tick(amber)
    await expect(html).toHaveAttribute('data-theme', 'amber')
    const amberCard = labelOf(amber)
    const amberLightTokens = await previewTokens(amberCard, 'light')
    const amberDarkTokens = await previewTokens(amberCard, 'dark')
    expect(amberLightTokens).not.toEqual(amberDarkTokens)
    expect(await activeColors()).toEqual(amberLightTokens)
    const amberPreviews = await previewColors(amberCard)
    expect(amberPreviews[0]).not.toEqual(amberPreviews[1])
    await expect(amberCard.locator('.palette-indicator')).toHaveCSS(
        'color',
        'rgb(11, 13, 17)',
    )
    expect(await stored(page)).toMatchObject({ palette: 'amber' })
    await page.keyboard.press('Escape')
    await expect(drawer).toBeHidden()
    await expect.poll(buttonColors).not.toEqual(defaultLight)

    const amberLight = await buttonColors()
    await page.emulateMedia({ colorScheme: 'dark' })
    await expect(html).toHaveClass(/\bdark\b/)
    await expect.poll(buttonColors).not.toEqual(amberLight)
    expect(await activeColors()).toEqual(amberDarkTokens)

    await page.goto('/settings/appearance')
    const pageGroup = page.getByRole('radiogroup', { name: '主题色' })
    const pageAmber = paletteOption(page, '琥珀橙')
    await expect(pageAmber).toBeChecked()
    await expect(labelOf(pageAmber).locator('.palette-indicator')).toHaveCSS(
        'color',
        'rgb(11, 13, 17)',
    )
    expect(await previewColors(labelOf(pageAmber))).toEqual(amberPreviews)
    await pageGroup.locator('xpath=..').screenshot({
        path: testInfo.outputPath('palettes-dark.png'),
        animations: 'disabled',
    })
    await page.emulateMedia({ colorScheme: 'light' })
    await expect(html).not.toHaveClass(/\bdark\b/)
    await pageGroup.locator('xpath=..').screenshot({
        path: testInfo.outputPath('palettes-light.png'),
        animations: 'disabled',
    })

    await page.setViewportSize({ width: 320, height: 720 })
    await page.goto('/settings/appearance')
    await expectPaletteColumns(pageGroup, 2)
    expect(
        await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
    ).toBe(true)
    await page.evaluate(() => {
        document.documentElement.style.fontSize = '200%'
    })
    await expectPaletteColumns(pageGroup, 1)
    await expect(pageAmber).toBeChecked()
    await page.evaluate(() => {
        sessionStorage.clear()
    })
})
