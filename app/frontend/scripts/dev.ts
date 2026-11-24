import { fileURLToPath } from 'node:url'
import { parseDevArguments } from '../config/development.ts'

try {
    const parsed = parseDevArguments(process.argv.slice(2))
    Object.assign(process.env, parsed.environment)
    process.chdir(fileURLToPath(new URL('..', import.meta.url)))
    const { runCLI } = await import('@rsbuild/core')
    runCLI({ argv: [...process.argv.slice(0, 2), 'dev', ...parsed.args] })
} catch (error) {
    console.error(
        error instanceof Error ? error.message : 'Unable to start frontend.',
    )
    process.exitCode = 1
}
