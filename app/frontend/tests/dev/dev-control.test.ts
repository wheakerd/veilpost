import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { once } from 'node:events'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { promisify } from 'node:util'

const exec = promisify(execFile)
const launcher = new URL('../../scripts/dev-control.ts', import.meta.url)
const command = `
    const [launcher, action, directory, port] = process.argv.slice(1)
    const { runFrontendDev } = await import(launcher)
    try {
        process.exitCode = await runFrontendDev(action, directory, Number(port))
    } catch (error) {
        console.error(error.message)
        process.exitCode = 1
    }
`

test(
    'background commands serialize starts, preserve ownership, and clean up failed launches',
    {
        skip: process.platform !== 'linux',
    },
    async (t) => {
        const root = await mkdtemp(join(tmpdir(), 'veilpost-dev-control-'))
        const directory = join(root, 'app/frontend')
        const devScript = join(directory, 'scripts/dev.ts')
        const stateFile = join(root, '.runtime/frontend-dev/process.json')
        const dependencies = join(directory, 'node_modules/@rsbuild/core')
        const listener = createServer((_request, response) => {
            response.end('port in use')
        })
        listener.listen(0, '127.0.0.1')
        await once(listener, 'listening')
        const endpoint = listener.address()
        assert.ok(endpoint && typeof endpoint !== 'string')
        const { port } = endpoint
        const run = (action: string) =>
            exec(process.execPath, [
                '--input-type=module',
                '--eval',
                command,
                launcher.href,
                action,
                directory,
                String(port),
            ])

        t.after(async () => {
            try {
                await run('stop')
            } finally {
                await rm(root, { force: true, recursive: true })
            }
        })

        try {
            await mkdir(join(directory, 'scripts'), { recursive: true })
            await mkdir(dependencies, { recursive: true })
            await writeFile(join(dependencies, 'package.json'), '{}\n')
            await writeFile(
                join(directory, '.nvmrc'),
                process.version.slice(1) + '\n',
            )
            await assert.rejects(run('start'), /is occupied/)
            const response = await fetch(`http://127.0.0.1:${port}`)
            assert.equal(await response.text(), 'port in use')
        } finally {
            listener.close()
            listener.closeAllConnections()
            await once(listener, 'close')
        }

        await writeFile(devScript, 'process.exitCode = 2\n')
        await assert.rejects(run('start'), /frontend exited/)
        await assert.rejects(readFile(stateFile), { code: 'ENOENT' })

        await writeFile(
            devScript,
            `
        import { createServer } from 'node:http'
        const port = Number(process.argv[process.argv.indexOf('--port') + 1])
        createServer((_request, response) => {
            response.end('<div id="root"></div>')
        }).listen(port, '127.0.0.1', () => {
            for (let line = 1; line <= 65; line++) console.log('fixture:', line)
        })
    `,
        )

        const [first, second] = await Promise.all([run('start'), run('start')])
        assert.match(first.stdout, /frontend: RUNNING/)
        assert.equal(first.stdout, second.stdout)
        const original = await readFile(stateFile, 'utf8')
        const record = JSON.parse(original)
        assert.equal(record.session, record.pid)
        const logs = (await run('logs')).stdout.trimEnd().split('\n')
        assert.equal(logs.length, 61)
        assert.equal(logs[1], 'fixture: 6')
        assert.equal(logs.at(-1), 'fixture: 65')

        await writeFile(
            stateFile,
            JSON.stringify({ ...record, start_ticks: '0' }),
        )
        try {
            await assert.rejects(run('status'), {
                code: 1,
                stdout: 'frontend: STOPPED\n',
            })
            assert.match((await run('stop')).stdout, /no owned process/)
            const response = await fetch(`http://127.0.0.1:${port}`)
            assert.match(await response.text(), /id="root"/)
        } finally {
            await writeFile(stateFile, original)
        }

        const restarted = await run('restart')
        assert.match(restarted.stdout, /frontend: RUNNING/)
        const newRecord = JSON.parse(await readFile(stateFile, 'utf8'))
        assert.notEqual(newRecord.pid, record.pid)
        assert.match((await run('status')).stdout, /frontend: RUNNING/)
        assert.match((await run('stop')).stdout, /STOPPED \(PID/)
        await assert.rejects(run('status'), {
            code: 1,
            stdout: 'frontend: STOPPED\n',
        })
    },
)
