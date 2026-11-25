import { expect, type Page } from '@playwright/test'
import { test, signIn, login } from './real-auth'
import { choose, chosen } from './element'

const preferencesKey = 'veilpost:preferences:v1'

const languages = [
    {
        browser: 'zh-CN',
        lang: 'zh-CN',
        login: '登录控制台',
        nodes: 'VPN 节点',
        description: '自建多 VPN 节点的统一控制面板',
    },
    {
        browser: 'en-US',
        lang: 'en',
        login: 'Sign in to the console',
        nodes: 'VPN nodes',
        description: 'One console for all your self-hosted VPN nodes',
    },
    {
        browser: 'zh-HK',
        lang: 'zh-TW',
        login: '登入控制台',
        nodes: 'VPN 節點',
        description: '自建多 VPN 節點的統一控制台',
    },
    {
        browser: 'ru-RU',
        lang: 'ru',
        login: 'Вход в консоль',
        nodes: 'VPN-узлы',
        description: 'Единая панель управления собственными VPN-узлами',
    },
] as const

// Browser storage apart from vue-router's own scroll positions, which the
// router keeps in sessionStorage independently of the language feature.
async function storage(page: Page) {
    const data = await page.evaluate(() => ({
        local: { ...localStorage },
        session: { ...sessionStorage },
    }))
    delete data.session['vue-router:scroll']
    return data
}

async function expectNoHorizontalOverflow(page: Page, where: string) {
    const overflow = await page.evaluate(() => {
        const issues: string[] = []
        if (document.documentElement.scrollWidth > window.innerWidth)
            issues.push(
                `page ${document.documentElement.scrollWidth} > ${window.innerWidth}`,
            )
        // Text controls must not clip their labels either.
        for (const element of document.querySelectorAll<HTMLElement>(
            'h1, h2, h3, .el-button, .el-tag, .nav-link, label, .language-select',
        )) {
            if (element.scrollWidth > element.clientWidth + 1)
                issues.push(
                    `${element.tagName.toLowerCase()}.${element.className}: ${element.textContent.trim()}`,
                )
        }
        return issues
    })
    expect(overflow, where).toEqual([])
}

for (const language of languages) {
    test.describe(`browser language ${language.browser}`, () => {
        test.use({ locale: language.browser })

        test('selects the interface language without writing storage', async ({
            page,
        }) => {
            await page.goto('/login')
            await expect(
                page.getByRole('heading', {
                    name: language.login,
                    exact: true,
                }),
            ).toBeVisible()
            await expect(page.locator('html')).toHaveAttribute(
                'lang',
                language.lang,
            )
            await expect(
                page.locator('meta[name="description"]'),
            ).toHaveAttribute('content', language.description)
            await expect(page).toHaveTitle(/ · Veilpost$/)
            // Following the browser language is the default and saves nothing.
            expect(await storage(page)).toEqual({ local: {}, session: {} })
        })

        test('login and console pages fit narrow screens', async ({
            page,
            account,
        }, testInfo) => {
            if (testInfo.project.name === 'mobile')
                await page.setViewportSize({ width: 320, height: 740 })
            await page.goto('/login')
            await expect(
                page.getByRole('heading', {
                    name: language.login,
                    exact: true,
                }),
            ).toBeVisible()
            await expectNoHorizontalOverflow(page, `${language.lang} /login`)
            await login(page, account, account.password, language.lang)
            await expect(page).toHaveURL('http://127.0.0.1:4173/')
            for (const path of [
                '/',
                '/nodes',
                '/settings/security',
                '/settings/appearance',
            ]) {
                await page.goto(path)
                await expect(page.locator('h1')).toBeVisible()
                await expect(page.locator('.route-content')).toHaveCSS(
                    'opacity',
                    '1',
                )
                await expect(page.locator('html')).toHaveAttribute(
                    'lang',
                    language.lang,
                )
                await expectNoHorizontalOverflow(
                    page,
                    `${language.lang} ${path}`,
                )
            }
            // Forms with the longest descriptions and buttons.
            await page.goto('/nodes')
            await page.locator('.page-intro .el-button').click()
            await expect(page.locator('#node-name')).toBeVisible()
            await expectNoHorizontalOverflow(
                page,
                `${language.lang} create node`,
            )
            await page.goto('/settings/security')
            await page.locator('.security-summary .el-button').click()
            await expect(
                page.locator(
                    '.security-form input[autocomplete="current-password"]',
                ),
            ).toBeVisible()
            await expectNoHorizontalOverflow(
                page,
                `${language.lang} enable 2FA`,
            )
            await page.screenshot({
                path: testInfo.outputPath(`security-${language.lang}.png`),
                fullPage: true,
                animations: 'disabled',
            })
        })
    })
}

test('switching the language updates the interface and survives reload', async ({
    page,
    account,
}) => {
    await signIn(page, account)
    await page.goto('/settings/appearance')
    await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN')
    const selector = page.getByRole('combobox', { name: '界面语言' })
    await expect(chosen(page, selector)).toContainText('跟随浏览器（简体中文）')

    await choose(page, selector, 'English')
    await expect(page.locator('html')).toHaveAttribute('lang', 'en')
    await expect(
        page.getByRole('heading', {
            name: 'Preferences',
            exact: true,
            level: 1,
        }),
    ).toBeVisible()
    await expect(
        page.getByRole('link', { name: 'VPN nodes', exact: true }),
    ).toBeVisible()
    await expect(page).toHaveTitle('Preferences · Veilpost')
    await expect(page.locator('meta[name="description"]')).toHaveAttribute(
        'content',
        'One console for all your self-hosted VPN nodes',
    )

    await page.reload()
    await expect(page.locator('html')).toHaveAttribute('lang', 'en')
    const english = page.getByRole('combobox', { name: 'Language' })
    await expect(chosen(page, english)).toContainText('English')
    await page
        .getByRole('link', { name: 'Account security', exact: true })
        .click()
    await expect(
        page.getByRole('heading', { name: 'Two-step verification' }),
    ).toBeVisible()
    await expect(page.getByText('Only this browser is signed in')).toBeVisible()

    await page.goto('/settings/appearance')
    await choose(
        page,
        page.getByRole('combobox', { name: 'Language' }),
        'Русский',
    )
    await expect(page.locator('html')).toHaveAttribute('lang', 'ru')
    await expect(
        page.getByRole('heading', {
            name: 'Настройки',
            exact: true,
            level: 1,
        }),
    ).toBeVisible()
    await choose(
        page,
        page.getByRole('combobox', { name: 'Язык интерфейса' }),
        '繁體中文',
    )
    await expect(page.locator('html')).toHaveAttribute('lang', 'zh-TW')
    await expect(
        page.getByRole('heading', { name: '偏好設定', exact: true, level: 1 }),
    ).toBeVisible()

    // Only the language choice is added to the existing preferences entry.
    const saved = await storage(page)
    expect(Object.keys(saved.local)).toEqual([preferencesKey])
    expect(JSON.parse(saved.local[preferencesKey])).toEqual({
        palette: 'abyss',
        mode: 'system',
        reducedMotion: false,
        locale: 'zh-TW',
        vision: 'standard',
        glassTint: 20,
        glassStrength: 60,
        pageTransition: 'fade',
        navMultiple: true,
        tabBar: true,
        progressBar: true,
        pageCache: true,
    })
    expect(saved.session).toEqual({})
    expect(JSON.stringify(saved)).not.toContain(account.password)

    // Following the browser again removes the saved language.
    await choose(
        page,
        page.getByRole('combobox', { name: '介面語言' }),
        '跟隨瀏覽器（简体中文）',
    )
    await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN')
    await page.reload()
    await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN')
    expect(
        JSON.parse((await storage(page)).local[preferencesKey]),
    ).not.toHaveProperty('locale')
})

test('restoring appearance defaults follows the browser language again', async ({
    page,
    account,
}) => {
    await signIn(page, account)
    await page.goto('/settings/appearance')
    await choose(
        page,
        page.getByRole('combobox', { name: '界面语言' }),
        'Русский',
    )
    await page.getByRole('button', { name: 'Сбросить всё' }).click()
    await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN')
    await expect(
        chosen(page, page.getByRole('combobox', { name: '界面语言' })),
    ).toContainText('跟随浏览器（简体中文）')
})

test.describe('invalid stored language', () => {
    test.use({ locale: 'en-GB' })

    test('falls back to the browser language', async ({ page }) => {
        await page.addInitScript((key) => {
            localStorage.setItem(key, JSON.stringify({ locale: 'de-DE' }))
        }, preferencesKey)
        await page.goto('/login')
        await expect(page.locator('html')).toHaveAttribute('lang', 'en')
        await expect(
            page.getByRole('heading', { name: 'Sign in to the console' }),
        ).toBeVisible()
    })
})

test.describe('error messages', () => {
    test.use({ locale: 'ru-RU' })

    test('backend error codes are shown in the current language', async ({
        page,
        account,
    }) => {
        await page.goto('/login')
        await login(page, account, `${account.password}-wrong`, 'ru')
        await expect(page.getByRole('alert')).toHaveText(
            'Неверное имя пользователя или пароль. Попробуйте ещё раз.',
        )
    })
})
