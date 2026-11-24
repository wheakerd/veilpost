import { defineConfig } from '@rsbuild/core'
import { pluginVue } from '@rsbuild/plugin-vue'
import { pluginSass } from '@rsbuild/plugin-sass'
import AutoImport from 'unplugin-auto-import/rspack'
import Icons from 'unplugin-icons/rspack'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'
import Components from 'unplugin-vue-components/rspack'
import { developmentOptions } from './config/development'
import packageJson from './package.json' with { type: 'json' }

export default defineConfig(({ command }) => {
    const development = developmentOptions(command, process.env)
    return {
        plugins: [pluginVue(), pluginSass()],
        tools: {
            rspack: {
                plugins: [
                    // Element Plus on demand: components used in templates and
                    // service APIs such as ElMessage are imported together with
                    // their styles, so no component ships unstyled.
                    AutoImport({
                        resolvers: [
                            ElementPlusResolver({ importStyle: 'css' }),
                        ],
                        dts: 'src/types/auto-imports.d.ts',
                        vueTemplate: false,
                    }),
                    Components({
                        resolvers: [
                            ElementPlusResolver({ importStyle: 'css' }),
                        ],
                        dirs: [],
                        dts: 'src/types/components.d.ts',
                    }),
                    // Icons compile from the locally installed @iconify-json
                    // sets at build time: no runtime fetch, no browser storage.
                    Icons({ compiler: 'vue3', autoInstall: false, scale: 1 }),
                ],
            },
        },
        dev: {
            // Build every route chunk up front: with lazy compilation a chunk
            // requested before it is built falls through to the History
            // fallback and the page fails with "Unexpected token '<'".
            lazyCompilation: false,
            hmr: true,
            liveReload: true,
            cliShortcuts: false,
            watchFiles: {
                paths: ['package.json', 'bun.lock', 'tsconfig.json'],
                type: 'restart',
            },
        },
        source: {
            entry: { index: './src/main.ts' },
            // vue-i18n feature flags: Composition API only, components imported
            // where used, no devtools in production builds.
            define: {
                __VUE_I18N_FULL_INSTALL__: JSON.stringify(false),
                __VUE_I18N_LEGACY_API__: JSON.stringify(false),
                __INTLIFY_PROD_DEVTOOLS__: JSON.stringify(false),
                // Shown in the console status line; not a secret.
                __APP_VERSION__: JSON.stringify(packageJson.version),
            },
        },
        output: {
            // Build output lives with the release archives under the
            // repository's dist/ directory, one subdirectory per product.
            distPath: { root: '../../dist/frontend' },
            cleanDistPath: true,
        },
        html: {
            template: './index.html',
            // The brand name only; the router sets a localized title at runtime.
            title: 'Veilpost',
            favicon: './public/favicon.svg',
        },
        server: {
            host: '127.0.0.1',
            port: 3000,
            strictPort: true,
            historyApiFallback: true,
            open: development.open,
            proxy: development.target
                ? {
                      '/api': {
                          target: development.target,
                          changeOrigin: false,
                          // Lets the backend rate-limit by browser address when it trusts this proxy.
                          xfwd: true,
                          proxyTimeout: 10_000,
                      },
                  }
                : undefined,
        },
    }
})
