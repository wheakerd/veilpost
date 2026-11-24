import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
    backendTarget,
    developmentOptions,
    parseDevArguments,
} from '../../config/development.ts'

test('backend arguments are consumed while Rsbuild arguments are preserved', () => {
    assert.deepEqual(
        parseDevArguments([
            '--host',
            '127.0.0.1',
            '--backend-host',
            '127.0.0.1',
            '--backend-port=49152',
            '--port',
            '3000',
            '--no-open',
        ]),
        {
            args: ['--host', '127.0.0.1', '--port', '3000'],
            environment: {
                DEV_BACKEND_HOST: '127.0.0.1',
                DEV_BACKEND_PORT: '49152',
                OPEN_BROWSER: 'false',
            },
        },
    )
    assert.equal(backendTarget('::1', '12345'), 'http://[::1]:12345')
})

test('invalid, partial, duplicated and URL-shaped backend addresses fail early', () => {
    for (const args of [
        ['--backend-host', '127.0.0.1'],
        ['--backend-port', '12345'],
        ['--backend-host'],
        ['--backend-host', '--port'],
        [
            '--backend-host=127.0.0.1',
            '--backend-host=127.0.0.1',
            '--backend-port=12345',
        ],
        ['--backend-host=http://user:secret@127.0.0.1', '--backend-port=12345'],
        ['--backend-host=0.0.0.0', '--backend-port=12345'],
        ['--backend-host=127.0.0.1', '--backend-port=0'],
        ['--backend-host=127.0.0.1', '--backend-port=65536'],
    ])
        assert.throws(() => parseDevArguments(args))
})

test('development opens the default browser, with explicit and CI opt-outs', () => {
    assert.equal(developmentOptions('dev', {}).open, true)
    assert.equal(
        developmentOptions('dev', { OPEN_BROWSER: 'false' }).open,
        false,
    )
    assert.equal(
        developmentOptions('dev', { CI: 'true', OPEN_BROWSER: 'true' }).open,
        false,
    )
    assert.equal(developmentOptions('dev', { CI: 'false' }).open, true)
})

test('build and preview ignore development endpoint and browser configuration', () => {
    for (const command of ['build', 'preview']) {
        assert.deepEqual(
            developmentOptions(command, {
                OPEN_BROWSER: 'true',
                DEV_BACKEND_HOST: 'invalid',
                DEV_BACKEND_PORT: '0',
            }),
            { open: false, target: undefined },
        )
    }
    assert.equal(
        developmentOptions('dev', {
            DEV_BACKEND_HOST: '127.0.0.1',
            DEV_BACKEND_PORT: '49152',
        }).target,
        'http://127.0.0.1:49152',
    )
})
