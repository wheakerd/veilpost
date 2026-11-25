# Veilpost frontend

English | [简体中文](README.zh-CN.md)

A Vue 3 + TypeScript + Rsbuild console that provides real backend sign-in, two-step verification, account security, a network overview, node enrollment and online status, the software store and configuration of installed software, certificates and DNS provider credentials, and preferences.

## Docker development

Run `make start` from the repository root. The frontend runs in a Compose container, source is bind-mounted for HMR, and `/api` is forwarded to the Go service through `backend:8080`. On first use, run `make admin-create` to create an administrator; there is no demo account.

The default entry point is <http://127.0.0.1:3000>. `make status` shows the actual port; `OPEN_BROWSER=false make start` turns off opening the browser on the host automatically. The container itself never tries to start a browser.

## Native development and command line

Use Node 26.13.1 from `.nvmrc` and Bun 1.4.4:

```bash
nvm use
bun install --frozen-lockfile
bun run dev --backend-host 127.0.0.1 --backend-port 49152
```

The backend host and port must be given together; an IP address or a service hostname is accepted. The development configuration reads the launcher arguments through private process variables and does not compile the address into browser assets. Without a backend address no mock data is generated, and the sign-in page reports that the service is unavailable.

Native `bun run dev` opens the system browser by default; `--no-open` or `OPEN_BROWSER=false` turns this off. CI and the production preview do not open a browser automatically. `make native-start` in the repository root can start the native backend, discover its random port, and pass it to the frontend; do not let it and Compose occupy frontend port 3000 at the same time.

On Linux with `flock` installed, `bun run dev:start` starts the frontend in the background. Use `bun run dev:status`, `bun run dev:logs`, `bun run dev:restart`, and `bun run dev:stop` to manage it. These TypeScript commands share the frontend process record and log with `make native-*` and automatically use the native backend address when it is ready.

```bash
bun run typecheck
bun run build
bun run format:check
bun run lint
bun run test:dev
PLAYWRIGHT_CHANNEL=chrome bun run test:e2e --workers=4
```

## Real APIs and tests

The nodes page calls same-origin `/api/nodes` to show the real node list and online / offline status (based on the last agent signed request that passed verification), and refreshes every 15 seconds while the page is visible. Creating and revoking nodes, reissuing enrollment tokens (unenrolled nodes only), and deleting nodes (revoked nodes only) all require entering the current password again. After creation, the page shows the enrollment command `veilpost-agent enroll --server <current Origin> --token-stdin` and the one-time enrollment token. The token is kept only in page memory and is cleared immediately when hidden, expired, when you leave the page, or when the session ends. With no nodes, the page keeps the "None connected" empty state and shows no sample data. The overview page reads the same list and shows how many non-revoked nodes are online. VPN configuration is not delivered yet. The nodes page no longer lists the protocols planned for integration and links to the software store instead. See the [node contract](docs/nodes-api.md) for API details.

The Store page (`/store`) calls same-origin `/api/apps` and lists the built-in catalog shipped with the console (Xray-core, Hysteria 2, AmneziaWG) with versions, protocols, ports, images pinned by digest, and installation state. Installing enables the software in the console, and its page (`/apps/:id`) then appears in the "Installed software" sidebar group; uninstalling requires entering the current password again and deletes all settings. The software page generates its settings form from the manifest; the server validates strictly and returns field-level errors. Sensitive settings are write-only and show only "Set / Not set", and entered values are kept only in page memory. "Deploy to nodes" shows a real not-yet-available state until node phase 2. Opening the address of software that is not installed shows "{name} isn’t installed" and points to the store. The bottom navigation on narrow screens keeps four items, and the entries for the store, certificates, DNS providers, and installed software appear below the brand. See the [store contract](docs/apps-api.md) for API details.

The Certificates page calls same-origin `/api/certificates`. It lists each certificate's domain names, issuer, status ("Issuing", "Valid", "Failed", "Expired", plus "Renewing" during renewal), expiry time, and last error. Requesting a certificate takes domain names, DNS provider credentials, an ACME service (Let's Encrypt by default, the staging environment, or a custom `https://` directory with optional EAB), and a contact email. Issuance runs in a backend background task and the API returns immediately; the page refreshes every 3 seconds while a task is in progress, and every 30 seconds otherwise. Certificates can be reissued, deleted, and downloaded as a certificate chain (PEM); private keys are not offered for browser download. The DNS providers page calls `/api/dns-credentials` and supports Cloudflare, Alibaba Cloud DNS, Tencent Cloud DNSPod, and AWS Route 53. Secrets are write-only, the list shows only "Set / Not set" for each field, and fields left empty when editing keep their values. Adding, changing, and deleting require the current password, and credentials used by a certificate cannot be deleted; "Verify" only runs read-only queries. Secrets, the EAB HMAC key, and passwords are kept only in page memory and are cleared when the form closes, when you leave the page, or when the session ends. See the [certificate contract](docs/certs-api.md) for API details.

The Account security page provides two-step verification setup and removal, recovery code regeneration and remaining count, password changes, and signing out other sessions. The client always calls same-origin `/api/auth` and accepts authentication state only after validating the response. Passwords, verification codes, recovery codes, and session tokens are not stored in localStorage, sessionStorage, or persisted Pinia. Actual authentication, authorization, TOTP, replay protection, recovery code consumption, and rate limiting are enforced by the Go backend.

Browser tests start a real Go service and a temporary SQLite database through `scripts/e2e-server.py`. Each test creates a random, isolated account through the local administrator command and then signs in over real HTTP. Verification codes are computed from the real setup secret. There is no MSW, no `page.route` request interception, and no test entry point in production, and temporary data is destroyed when the tests finish.

The test port is 4173, and the backend port is assigned by the system. Tests cover, on desktop and mobile, sign-in, refresh, protected routes, real two-step verification, single use of recovery codes, expiry, network failures, theme preferences, keyboard operation, enlarged text, and interface language (choosing the default language from the browser language, keeping the choice after refresh, `<html lang>`, and no horizontal overflow on narrow screens in all four languages). Playwright projects pin `locale: 'zh-CN'`, so other tests see the Simplified Chinese interface; language tests use `test.use({ locale })` to simulate other browser languages. Certificate tests use only a custom ACME directory pointing to a closed local port, so the background task really fails with "The ACME service could not be reached"; they do not contact public ACME services or DNS providers and do not click "Verify". Node tests create and revoke nodes through the real API and verify online status by calling the real enrollment and signed polling endpoints with Ed25519 from the test process; they do not run the `veilpost-agent` program. The tests share one temporary backend, and the node list may contain nodes created by other tests running in parallel, so each test checks only the nodes it created. Software installation state is global, and the desktop and mobile projects run the same test at the same time: in `tests/e2e/apps.spec.ts` each test owns one piece of software (by creating a lock directory in this run's temporary data directory), resets that software to the state it needs through the real API at the start, and asserts only on that software; other test files do not install software. The first time you use the browser bundled with Playwright, run `bunx playwright install chromium`.

## Loading and interaction

- The initial session restoration shows a loading card and does not reveal protected content early.
- The global request bar follows real request state; sign-in and verification buttons show the action in progress and prevent duplicate submission.
- Service connection failures offer a retry entry, and error and success feedback uses short, non-bouncing transitions.
- Security setup step changes and page navigation use interruptible Motion animations; displacement is removed when the user or the system turns on reduced motion.
- Inline forms and confirmations open and fold from their current size (`src/components/Reveal.vue`; table rows cross-fade); drawers and the narrow-screen sheet move on the iOS sheet curve; the top bar shows its hairline only while content passes beneath it; a node that has just come online sends out one ring from its status dot.
- Signing in reports Caps Lock beside the password label, shakes the form once when credentials or a code are rejected, draws the authenticator code as six cells that lift once when it passes, holds "Verified, continuing…" briefly, and then dissolves the card while the globe gathers behind "Entering the console…" (`src/composables/useSignInExit.ts`).
- Unsaved edits in the node, DNS credential, certificate and password forms ask in place before Cancel or Escape drops them (`src/composables/useDiscardGuard.ts`); a control that cannot be used explains why on hover and focus (`src/components/DisabledReason.vue`); a loading state appears only after 180 ms.
- Reduced transparency, high contrast, system fonts, 200% text, and the mobile bottom navigation are still supported.

## Localization

The interface supports 简体中文 (`zh-CN`), English (`en`), 繁體中文 (`zh-TW`), and Русский (`ru`), using the Composition API mode of `vue-i18n`.

- By default the language is matched from the browser languages (`navigator.languages`): `zh-TW`, `zh-HK`, `zh-MO`, and `zh-Hant` map to Traditional Chinese, any other `zh` to Simplified Chinese, `ru` to Russian, and `en` to English; when none match, Simplified Chinese is used.
- "Preferences" lets you choose a language or go back to "Browser default". Only an explicitly chosen language is written to the persisted preferences in `src/stores/preferences.ts` (the `locale` field); following the browser writes nothing, and "Restore all defaults" also returns to following the browser.
- `<html lang>`, the page title, the page description, and the Element Plus language switch with the interface language; dates, relative times, and numbers are formatted with `Intl` for the current language (`src/i18n/format.ts`).
- Language packs are split by feature in `src/locales/<locale>/<feature>.ts` (`common`, `errors`, `auth`, `overview`, `nodes`, `security`, `appearance`, `apps`, `certs`). The names, descriptions, and setting labels referenced by the software catalog manifests live under `apps.catalog.<id>`, and `tests/dev/locales.test.ts` checks that every key in the manifests exists. Simplified Chinese is the type source and the other languages are checked with `satisfies`, so missing or extra keys make `bun run typecheck` fail; at runtime, missing messages fall back to Simplified Chinese, and development builds warn in the console.
- `src/locales/<locale>/errors.ts` is keyed by backend error code; a new error code needs text in all four languages.
- Messages that need plurals separate forms with `|`: two forms for English (one | other), three for Russian (one | few | many), and none for Chinese. Code fragments in messages (commands, paths) are rendered through `I18nT` slots and are not written into the language packs.
- `tests/dev/locales.test.ts`, part of `bun run test:dev`, checks that each language has the same files and keys, that placeholders match, that keys referenced in the source exist, and that there is no Chinese, Japanese, or Russian interface text outside the language packs.

## Theme and structure

The interface is Liquid Glass after iOS 26 and macOS 26 (`src/styles/_glass.scss`, `src/theme/liquidGlass.ts`). The workspace stands on a still backdrop of the palette's primary and secondary; the sidebar, top bar, tab bar and page panel are panes of glass half a rem apart, and the top bar lifts once content scrolls beneath it. Panels and cards on the page panel are hairline frames with corner ticks, numbers use a monospace font, and the black-and-yellow hazard strip appears only where attention is needed. Menus, popovers, drawers, dialogs, messages and the sign-in card are clearer glass with edge lights and, in Chromium-based browsers, a refracting rim; they grow out of their trigger as their glass thickens. Every button is the same filled capsule of the primary color (danger actions in red) with a light that follows the pointer and a spring-loaded press; single-line fields are capsules. Glass turns solid under reduced transparency or increased contrast, and the press and refraction stop under reduced motion. On narrow screens the tab bar's selection is a glass lens: pressing a tab lifts it, dragging moves it across the tabs, and releasing throws it to the nearest tab in the direction of the drag and opens that page. The sign-in page draws a slowly rotating wireframe globe on a Canvas (`src/components/LabGlobe.vue`), which stops when the page is in the background or reduced motion is on. Text is set in rem and the root size follows the viewport (`src/styles/_type.scss`): 15px up to about 1000px wide, 16px at 1440px and at most 18px from 2560px, with Element Plus text and control sizes following it; the reader's default font size and page zoom still scale everything. On wide screens the window never scrolls: the top bar and the sidebar stay fixed and only the page panel scrolls, keeping each page's place when it is left and reopened (`src/router/pageCache.ts`); narrow screens scroll the window. The console shell has a full-width top bar (the brand and the sidebar toggle on the left; refresh, light-dark, language, preferences, clock and the account trigger on the right), an Element Plus menu as the sidebar with folding groups that collapses to an icon rail, and an "Open pages" tab bar of capsules that exists only in memory (`src/stores/tabs.ts`, desktop widths only) and can be hidden from its own close button. The account trigger opens the account center (`src/components/AccountCenter.vue`), a dialog with the signed-in account, its two-step verification state, Account security, Preferences and Sign out. Switching light and dark spreads the new appearance out of the pressed point as a circle and gathers it back, through the View Transitions API where the browser has it (`src/theme/index.ts`, `src/styles/_theme-transition.scss`); otherwise, and under reduced motion, it switches at once.

"Preferences" is both a page and a drawer that opens from the button on the right of the top bar (on narrow screens a sheet from the bottom edge: drag its header down, or flick it, to close): eight theme colors (Deep Sea, the default, Clear Sky, Emerald, Sea Mist, Amber, Coral, Rose, Ultraviolet), each with button text chosen to keep 4.5:1; Light, Dark, System; vision mode (Standard, Color-safe, Grayscale); the tint and strength of liquid glass; four page transitions; sidebar width (adjusted by dragging the right edge of the sidebar); sidebar groups expanding together or one at a time; switches for the tab bar, the loading bar, page state caching, and reduced motion; and the interface language. Only these non-sensitive UI settings are persisted (the collapsed sidebar and a custom width are written only when set). Page state caching keeps only the pages of open tabs; passwords, tokens, and keys are still cleared when you leave a page, and the Account security page is not cached. Element Plus is imported on demand through `unplugin-vue-components` and `unplugin-auto-import`, and component styles load automatically with their components. Theme tokens are in `src/styles/_themes.scss`, palette metadata in `src/theme/palettes.ts`, and palette names in each language's `appearance.ts`. Icons come from `@iconify-json/lucide` and are compiled into components by `unplugin-icons` at build time (`~icons/lucide/<name>`); nothing is fetched from the network at runtime or written to browser storage.

Only TypeScript and Vue are used; do not add `.js` / `.mjs` files. The launcher `scripts/dev.ts` is run directly by Node. `tsconfig.json` enables strict mode and `noUncheckedIndexedAccess`. `eslint.config.ts` (typescript-eslint strict type-checked rules, eslint-plugin-vue, eslint-config-prettier) and Prettier divide the work: `bun run format` runs `eslint --fix` and then Prettier, and `bun run lint` only checks. Build output goes to `dist/frontend/` in the repository root.

```text
src/api/          Real authentication, node, store, and certificate clients, response validation, request activity state
src/stores/       In-memory authentication state, in-memory software catalog, and non-sensitive preferences
src/components/   Navigation, loading, feedback, authentication layout
src/views/        Sign-in, verification, account security, overview, nodes, store, software pages, certificates, DNS providers, and preferences
src/i18n/         vue-i18n instance, browser language matching, and Intl formatting
src/locales/      Language packs split by language and feature
config/           Development proxy and browser switch
scripts/          Rsbuild launcher and background development commands
tests/e2e/        Real-backend browser tests
tests/dev/        Development option and language pack checks
```

The development server has lazy compilation turned off and compiles all route chunks at startup, so that chunk requests on first visit do not fall through to the History fallback page. Deploying the `dist/frontend/` static assets requires a History fallback and a `/api` reverse proxy. `bun run preview` does not use the development backend address; it is only for previewing the static build. See the [authentication contract](docs/auth-api.md), [node contract](docs/nodes-api.md), [store contract](docs/apps-api.md), and [certificate contract](docs/certs-api.md) for API details, and [Development and releases](../../docs/releasing.md) for release steps.
