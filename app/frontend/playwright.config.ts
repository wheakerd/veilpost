import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
    testDir: './tests/e2e',
    fullyParallel: true,
    forbidOnly: Boolean(process.env.CI),
    retries: process.env.CI ? 1 : 0,
    reporter: 'list',
    use: {
        baseURL: 'http://127.0.0.1:4173',
        trace: 'retain-on-failure',
        // Optional installed Chrome; default is Playwright's bundled Chromium.
        channel: process.env.PLAYWRIGHT_CHANNEL || undefined,
    },
    projects: [
        {
            name: 'desktop',
            use: {
                ...devices['Desktop Chrome'],
                // The interface follows the browser language; tests that do not set
                // their own locale see Simplified Chinese.
                locale: 'zh-CN',
                viewport: { width: 1440, height: 1000 },
            },
        },
        {
            name: 'mobile',
            use: {
                ...devices['Desktop Chrome'],
                locale: 'zh-CN',
                viewport: { width: 390, height: 844 },
                isMobile: true,
                hasTouch: true,
            },
        },
    ],
    webServer: {
        command: 'python3 ../../scripts/e2e-server.py',
        url: 'http://127.0.0.1:4173',
        env: { OPEN_BROWSER: 'false' },
        timeout: 180_000,
        reuseExistingServer: false,
    },
})
