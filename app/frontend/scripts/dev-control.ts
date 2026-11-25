import { execFileSync, spawn } from 'node:child_process'
import { randomUUID } from 'node:crypto'
import { once } from 'node:events'
import {
    closeSync,
    existsSync,
    fstatSync,
    mkdirSync,
    openSync,
    readFileSync,
    realpathSync,
    renameSync,
    rmSync,
    statSync,
    writeFileSync,
    writeSync,
} from 'node:fs'
import { open } from 'node:fs/promises'
import { get } from 'node:http'
import type { IncomingMessage } from 'node:http'
import { createConnection, isIP } from 'node:net'
import { join } from 'node:path'
import { text } from 'node:stream/consumers'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'
import { isDeepStrictEqual } from 'node:util'
import packageJson from '../package.json' with { type: 'json' }

const loopback = '127.0.0.1'
const actions = ['start', 'stop', 'restart', 'status', 'logs']
const noProcess = ['ENOENT', 'ESRCH', 'EACCES', 'EPERM']

interface ProcessRecord {
    pid: number
    start_ticks?: string
    session?: number
    cwd?: string
    ip?: string
    port?: number
    parameters?: unknown
}

interface FrontendRuntime {
    directory: string
    runtime: string
    stateFile: string
    logFile: string
    port: number
}

function errorCode(error: unknown) {
    return (error as NodeJS.ErrnoException | null)?.code ?? ''
}

function readRecord(file: string): ProcessRecord | undefined {
    let record: unknown
    try {
        record = JSON.parse(readFileSync(file, 'utf8'))
    } catch (error) {
        if (errorCode(error) === 'ENOENT') return
        throw error
    }
    if (typeof record !== 'object' || record === null || !('pid' in record)) {
        throw new Error(`Invalid process record: ${file}`)
    }
    const { pid } = record
    const port = 'port' in record ? record.port : undefined
    const ip = 'ip' in record ? record.ip : undefined
    if (typeof pid !== 'number' || !Number.isInteger(pid) || pid <= 1) {
        throw new Error(`Invalid process record: ${file}`)
    }
    if (
        port !== undefined &&
        (typeof port !== 'number' ||
            !Number.isInteger(port) ||
            port < 1 ||
            port > 65535)
    ) {
        throw new Error(`Invalid process port: ${file}`)
    }
    if (ip !== undefined && (typeof ip !== 'string' || !isIP(ip))) {
        throw new Error(`Invalid process address: ${file}`)
    }
    return { ...record, pid, port, ip }
}

function processIdentity(pid: number): ProcessRecord | undefined {
    try {
        const stat = readFileSync(`/proc/${pid}/stat`, 'utf8')
        // The name in parentheses can itself contain spaces and parentheses.
        const fields = stat.slice(stat.lastIndexOf(')') + 2).split(' ')
        const startTicks = fields[19]
        if (!startTicks || fields[0] === 'Z') return
        return {
            pid,
            start_ticks: startTicks,
            session: Number(fields[3]),
            cwd: realpathSync(`/proc/${pid}/cwd`),
        }
    } catch (error) {
        if (noProcess.includes(errorCode(error))) return
        throw error
    }
}

function isOwned(record: ProcessRecord, directory: string) {
    if (record.cwd !== directory || record.session !== record.pid) {
        return false
    }
    const current = processIdentity(record.pid)
    if (!current) return false
    return (
        current.start_ticks === record.start_ticks &&
        current.session === record.session &&
        current.cwd === record.cwd
    )
}

function address(port: number, ip = loopback) {
    const host = isIP(ip) === 6 ? `[${ip}]` : ip
    return `http://${host}:${port}`
}

async function httpReady(url: string, check: (body: string) => boolean) {
    try {
        const response = await new Promise<IncomingMessage>(
            (resolve, reject) => {
                const signal = AbortSignal.timeout(1000)
                get(url, { agent: false, signal }, resolve).on('error', reject)
            },
        )
        const body = await text(response)
        return response.statusCode === 200 && check(body)
    } catch {
        return false
    }
}

async function backendParameters(frontend: FrontendRuntime) {
    const record = readRecord(
        join(frontend.runtime, 'backend-dev/process.json'),
    )
    if (
        !record?.port ||
        !isOwned(record, realpathSync(join(frontend.directory, '../backend')))
    ) {
        return {}
    }
    const ip = record.ip ?? loopback
    const ready = await httpReady(
        address(record.port, ip) + '/api/health',
        (body) =>
            isDeepStrictEqual(JSON.parse(body), {
                status: 'ok',
                service: 'veilpost',
            }),
    )
    return ready ? { backend_host: ip, backend_port: record.port } : {}
}

function writeRecord(file: string, state: ProcessRecord) {
    const temporary = `${file}.${randomUUID()}`
    try {
        writeFileSync(temporary, JSON.stringify(state, null, 2) + '\n', {
            flag: 'wx',
            mode: 0o600,
        })
        renameSync(temporary, file)
    } finally {
        rmSync(temporary, { force: true })
    }
}

function checkTools(directory: string) {
    const nodeVersion = readFileSync(join(directory, '.nvmrc'), 'utf8').trim()
    if (process.version !== `v${nodeVersion}`) {
        throw new Error(
            `Node ${nodeVersion} required, found ${process.version}.`,
        )
    }
    const bunVersion = packageJson.packageManager.split('@')[1]
    const installedBun = execFileSync('bun', ['--version'], {
        encoding: 'utf8',
    }).trim()
    if (installedBun !== bunVersion) {
        throw new Error(`Bun ${bunVersion} required, found ${installedBun}.`)
    }
    if (
        !existsSync(join(directory, 'node_modules/@rsbuild/core/package.json'))
    ) {
        throw new Error('Frontend dependencies missing; run make install.')
    }
}

function portInUse(port: number) {
    return new Promise<boolean>((resolve) => {
        const socket = createConnection({ host: loopback, port })
        const finish = (connected: boolean) => {
            socket.destroy()
            resolve(connected)
        }
        socket.setTimeout(1000)
        socket.once('connect', () => {
            finish(true)
        })
        socket.once('timeout', () => {
            finish(false)
        })
        socket.once('error', () => {
            finish(false)
        })
    })
}

function frontendReady(url: string) {
    return httpReady(url, (body) => body.includes('id="root"'))
}

function printStatus(
    frontend: FrontendRuntime,
    state: ProcessRecord,
    ready: boolean,
) {
    const url = address(state.port ?? frontend.port, state.ip)
    console.log(
        `frontend: ${ready ? 'RUNNING' : 'NOT READY'} ${url} (PID ${state.pid})`,
    )
    console.log(`Log: ${frontend.logFile}`)
}

async function stop(
    frontend: FrontendRuntime,
    state: ProcessRecord | undefined,
) {
    const owned = state && isOwned(state, frontend.directory)
    if (owned) {
        for (const [signal, wait] of [
            ['SIGTERM', 12_000],
            ['SIGKILL', 3000],
        ] as const) {
            if (!isOwned(state, frontend.directory)) break
            try {
                process.kill(-state.pid, signal)
            } catch (error) {
                if (errorCode(error) !== 'ESRCH') throw error
                break
            }
            const deadline = performance.now() + wait
            while (
                isOwned(state, frontend.directory) &&
                performance.now() < deadline
            ) {
                await delay(50)
            }
        }
        if (isOwned(state, frontend.directory)) {
            throw new Error('Failed to stop frontend; process record retained.')
        }
    }
    rmSync(frontend.stateFile, { force: true })
    console.log(
        `frontend: STOPPED (${owned ? `PID ${state.pid}` : 'no owned process'})`,
    )
}

async function start(
    frontend: FrontendRuntime,
    existing: ProcessRecord | undefined,
    signal?: AbortSignal,
) {
    const parameters = await backendParameters(frontend)
    const running = existing && isOwned(existing, frontend.directory)
    signal?.throwIfAborted()
    if (running && isDeepStrictEqual(existing.parameters ?? {}, parameters)) {
        const url = address(existing.port ?? frontend.port, existing.ip)
        if (!(await frontendReady(url))) {
            throw new Error(
                `frontend is running but not ready; inspect ${frontend.logFile}`,
            )
        }
        printStatus(frontend, existing, true)
        return
    }
    checkTools(frontend.directory)
    if (running) await stop(frontend, existing)
    const url = address(frontend.port)
    if (await portInUse(frontend.port)) {
        throw new Error(
            `${url} is occupied; the other process was left untouched.`,
        )
    }
    const env = { ...process.env }
    delete env.DEV_BACKEND_HOST
    delete env.DEV_BACKEND_PORT
    const args = [
        'scripts/dev.ts',
        '--host',
        loopback,
        '--port',
        String(frontend.port),
    ]
    if (parameters.backend_host && parameters.backend_port) {
        args.push(
            '--backend-host',
            parameters.backend_host,
            '--backend-port',
            String(parameters.backend_port),
        )
    }
    signal?.throwIfAborted()
    const log = openSync(frontend.logFile, 'a', 0o600)
    let child
    try {
        writeSync(log, `\n--- start ${new Date().toISOString()} ---\n`)
        child = spawn(process.execPath, args, {
            cwd: frontend.directory,
            env,
            detached: true,
            stdio: ['ignore', log, log],
        })
    } finally {
        closeSync(log)
    }
    await once(child, 'spawn')
    child.unref()
    const identity = child.pid ? processIdentity(child.pid) : undefined
    if (!identity) {
        throw new Error(
            `frontend exited immediately; inspect ${frontend.logFile}`,
        )
    }
    const state = { ...identity, port: frontend.port, parameters }
    try {
        writeRecord(frontend.stateFile, state)
        const deadline = performance.now() + 25_000
        while (performance.now() < deadline) {
            signal?.throwIfAborted()
            if (!isOwned(state, frontend.directory)) {
                throw new Error(`frontend exited; inspect ${frontend.logFile}`)
            }
            const ready = await frontendReady(url)
            signal?.throwIfAborted()
            if (ready && isOwned(state, frontend.directory)) {
                printStatus(frontend, state, true)
                return
            }
            await delay(100, undefined, { signal })
        }
        throw new Error(
            `frontend did not become ready; inspect ${frontend.logFile}`,
        )
    } catch (error) {
        await stop(frontend, state)
        throw error
    }
}

async function logs(file: string) {
    console.log('frontend log:')
    const log = await open(file).catch((error: unknown) => {
        if (errorCode(error) === 'ENOENT') return
        throw error
    })
    if (!log) {
        console.log('frontend: no log yet.')
        return
    }
    try {
        const lines: string[] = []
        for await (const line of log.readLines()) {
            lines.push(line)
            if (lines.length > 60) lines.shift()
        }
        if (lines.length) console.log(lines.join('\n'))
    } finally {
        await log.close()
    }
}

export async function runFrontendDev(
    action: string,
    directory: string,
    port: number,
    signal?: AbortSignal,
    backendLock?: number,
) {
    if (!actions.includes(action)) {
        throw new Error('Usage: dev-control.ts start|stop|restart|status|logs')
    }
    if (process.platform !== 'linux') {
        throw new Error('Background development commands require Linux.')
    }
    directory = realpathSync(directory)
    const runtime = join(directory, '../../.runtime')
    const frontend: FrontendRuntime = {
        directory,
        port,
        runtime,
        stateFile: join(runtime, 'frontend-dev/process.json'),
        logFile: join(runtime, 'frontend-dev/server.log'),
    }
    const locks: number[] = []
    try {
        // Match the lock files and order used by the native Make targets.
        for (const name of ['backend', 'frontend']) {
            const folder = join(runtime, `${name}-dev`)
            mkdirSync(folder, { recursive: true, mode: 0o700 })
            const path = join(folder, 'control.lock')
            const inherited = name === 'backend' ? backendLock : undefined
            const lock = inherited ?? openSync(path, 'a', 0o600)
            if (inherited === undefined) {
                locks.push(lock)
            } else {
                const expected = statSync(path, { bigint: true })
                const received = fstatSync(lock, { bigint: true })
                if (
                    received.dev !== expected.dev ||
                    received.ino !== expected.ino
                ) {
                    throw new Error(
                        'Expected the native backend lock on standard input.',
                    )
                }
            }
            // flock locks the open file description shared with this process.
            const command = spawn('flock', ['-x', '3'], {
                signal,
                stdio: ['ignore', 'ignore', 'inherit', lock],
            })
            const result: unknown[] = await once(command, 'exit')
            if (result[0] !== 0) throw new Error(`Could not lock ${folder}.`)
        }
        signal?.throwIfAborted()
        if (action === 'logs') {
            await logs(frontend.logFile)
            return 0
        }
        const state = readRecord(frontend.stateFile)
        if (action === 'status') {
            if (!state || !isOwned(state, directory)) {
                console.log('frontend: STOPPED')
                return 1
            }
            const ready = await frontendReady(
                address(state.port ?? port, state.ip),
            )
            printStatus(frontend, state, ready)
            return ready ? 0 : 1
        }
        if (action === 'stop' || action === 'restart')
            await stop(frontend, state)
        if (action === 'start' || action === 'restart') {
            await start(
                frontend,
                action === 'start' ? state : undefined,
                signal,
            )
        }
        return 0
    } finally {
        for (const lock of locks.reverse()) closeSync(lock)
    }
}

if (import.meta.main) {
    const controller = new AbortController()
    const interrupt = () => {
        controller.abort(new Error('Frontend command interrupted.'))
    }
    process.once('SIGINT', interrupt)
    process.once('SIGTERM', interrupt)
    try {
        const [action = '', ...args] = process.argv.slice(2)
        const inherited = args.length === 1 && args[0] === '--backend-lock'
        process.exitCode = await runFrontendDev(
            args.length && !inherited ? '' : action,
            fileURLToPath(new URL('..', import.meta.url)),
            3000,
            controller.signal,
            // The lock stays held while this command runs. Managed servers
            // already use /dev/null for stdin, so they never inherit it.
            inherited ? 0 : undefined,
        )
    } catch (error) {
        console.error('ERROR:', error instanceof Error ? error.message : error)
        process.exitCode = 1
    } finally {
        process.off('SIGINT', interrupt)
        process.off('SIGTERM', interrupt)
    }
}
