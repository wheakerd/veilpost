import assert from 'node:assert/strict'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, resolve } from 'node:path'
import { test } from 'node:test'
import {
    defaultLocale,
    matchLocale,
    supportedLocales,
} from '../../src/i18n/locales.ts'

// Static checks for the language packs in src/locales. Key parity is also
// enforced by `bun run typecheck`; these tests add what types cannot see.

const root = resolve(import.meta.dirname, '../..')
const localesDirectory = join(root, 'src/locales')
type Messages = { [key: string]: string | Messages }

const features = readdirSync(join(localesDirectory, defaultLocale))
    .filter((file) => file.endsWith('.ts') && file !== 'index.ts')
    .map((file) => file.slice(0, -3))
    .sort()

async function loadPack(locale: string) {
    const pack: Record<string, Messages> = {}
    for (const feature of features) {
        const module = await import(
            join(localesDirectory, locale, `${feature}.ts`)
        )
        pack[feature] = module.default
    }
    return pack
}

function flatten(messages: Messages, prefix = ''): Map<string, string> {
    const entries = new Map<string, string>()
    for (const [key, value] of Object.entries(messages)) {
        const path = prefix ? `${prefix}.${key}` : key
        if (typeof value === 'string') entries.set(path, value)
        else for (const entry of flatten(value, path)) entries.set(...entry)
    }
    return entries
}

function placeholders(message: string) {
    return [...new Set(message.match(/\{[A-Za-z]+\}/g) ?? [])].sort()
}

function sourceFiles(directory: string): string[] {
    return readdirSync(directory).flatMap((name) => {
        const path = join(directory, name)
        if (statSync(path).isDirectory()) return sourceFiles(path)
        return /\.(ts|vue)$/.test(name) ? [path] : []
    })
}

test('browser languages map to the supported interface languages', () => {
    const cases: [string[], string][] = [
        [['zh-TW'], 'zh-TW'],
        [['zh-HK', 'en'], 'zh-TW'],
        [['zh-Hant'], 'zh-TW'],
        [['zh-Hant-CN'], 'zh-TW'],
        [['zh-MO'], 'zh-TW'],
        [['zh-Hans-HK'], 'zh-CN'],
        [['zh-CN'], 'zh-CN'],
        [['zh'], 'zh-CN'],
        [['zh-SG'], 'zh-CN'],
        [['ru-RU'], 'ru'],
        [['ru'], 'ru'],
        [['en-US'], 'en'],
        [['en-GB', 'ru'], 'en'],
        [['de-DE', 'fr', 'ru-UA'], 'ru'],
        [['de-DE', 'ja'], 'zh-CN'],
        [[], 'zh-CN'],
    ]
    for (const [languages, expected] of cases)
        assert.equal(matchLocale(languages), expected, languages.join(','))
})

test('every language has the same feature files', () => {
    for (const locale of supportedLocales) {
        const files = readdirSync(join(localesDirectory, locale))
            .filter((file) => file !== 'index.ts')
            .map((file) => file.replace(/\.ts$/, ''))
            .sort()
        assert.deepEqual(files, features, locale)
    }
})

test('translations keep the keys and placeholders of Simplified Chinese', async () => {
    const source = flatten(await loadPack(defaultLocale))
    for (const locale of supportedLocales) {
        const messages = flatten(await loadPack(locale))
        assert.deepEqual([...messages.keys()].sort(), [...source.keys()].sort())
        for (const [key, message] of messages) {
            assert.ok(message.trim(), `${locale} ${key} is empty`)
            // "@" starts a linked message in vue-i18n; none are used.
            assert.ok(!message.includes('@'), `${locale} ${key} contains "@"`)
            const choices = message.split('|')
            assert.ok(
                choices.length <=
                    (locale === 'ru' ? 3 : locale === 'en' ? 2 : 1),
                `${locale} ${key} has too many plural forms`,
            )
            for (const choice of choices)
                assert.deepEqual(
                    placeholders(choice),
                    placeholders(source.get(key)!),
                    `${locale} ${key} placeholders`,
                )
        }
    }
})

test('message keys used in the source exist', async () => {
    const messages = flatten(await loadPack(defaultLocale))
    const prefixes = new Set(
        [...messages.keys()].flatMap((key) =>
            key
                .split('.')
                .map((_, index, parts) => parts.slice(0, index).join('.')),
        ),
    )
    let checked = 0
    for (const file of sourceFiles(join(root, 'src'))) {
        if (file.startsWith(localesDirectory)) continue
        const text = readFileSync(file, 'utf8')
        for (const match of text.matchAll(
            /(?:\bt\(\s*|keypath=")[`']([\w.]+)(\$\{)?/g,
        )) {
            const [, key = '', dynamic] = match
            const name = `${relative(root, file)}: ${key}`
            if (dynamic) assert.ok(prefixes.has(key.replace(/\.$/, '')), name)
            else assert.ok(messages.has(key), name)
            checked++
        }
        for (const match of text.matchAll(
            /'(errors|common|auth|overview|nodes|security|appearance|apps|certs)\.[\w.]+'/g,
        )) {
            const key = match[0].slice(1, -1)
            assert.ok(messages.has(key), `${relative(root, file)}: ${key}`)
        }
    }
    assert.ok(checked > 100, `only ${checked} keys found`)
})

test('every key named by the app catalog exists', async () => {
    // The backend manifests reference the packs by key (src/api/apps.ts).
    const messages = flatten(await loadPack(defaultLocale))
    const catalog = resolve(root, '../backend/internal/apps/catalog')
    const manifests = readdirSync(catalog).filter((file) =>
        file.endsWith('.json'),
    )
    assert.ok(manifests.length >= 3, 'no manifests found')
    for (const file of manifests) {
        const manifest = JSON.parse(readFileSync(join(catalog, file), 'utf8'))
        const keys = [manifest.name, manifest.description]
        for (const setting of manifest.settings) {
            keys.push(setting.label)
            if (setting.hint) keys.push(setting.hint)
            for (const option of setting.options ?? []) keys.push(option.label)
        }
        for (const key of keys) assert.ok(messages.has(key), `${file}: ${key}`)
    }
    // Every catalog message belongs to a manifest that still exists.
    const ids = new Set(manifests.map((file) => file.replace(/\.json$/, '')))
    for (const key of messages.keys())
        if (key.startsWith('apps.catalog.'))
            assert.ok(ids.has(key.split('.')[2]!), key)
})

test('interface text lives only in the language packs', () => {
    // Han, kana and Cyrillic characters outside the packs indicate hard-coded
    // interface text. Comments are not checked; keep them in English.
    const files = [
        ...sourceFiles(join(root, 'src')).filter(
            (file) => !file.startsWith(localesDirectory),
        ),
        join(root, 'index.html'),
        join(root, 'rsbuild.config.ts'),
    ]
    for (const file of files) {
        const lines = readFileSync(file, 'utf8').split('\n')
        lines.forEach((line, index) => {
            assert.ok(
                !/[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Cyrillic}]/u.test(
                    line,
                ),
                `${relative(root, file)}:${index + 1}: ${line.trim()}`,
            )
        })
    }
})
