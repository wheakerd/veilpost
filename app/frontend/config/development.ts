import { isIP } from 'node:net'

export function backendTarget(host?: string, port?: string) {
    if (host === undefined && port === undefined) return undefined
    if (
        !host ||
        (!isIP(host) &&
            !/^(?=.{1,253}$)[a-zA-Z0-9](?:[a-zA-Z0-9.-]*[a-zA-Z0-9])?$/.test(
                host,
            )) ||
        host === '0.0.0.0' ||
        host === '::' ||
        !port ||
        !/^[0-9]+$/.test(port) ||
        Number(port) < 1 ||
        Number(port) > 65535
    ) {
        throw new Error(
            'Backend requires a host name or IP address and a port from 1 to 65535.',
        )
    }
    return `http://${isIP(host) === 6 ? `[${host}]` : host}:${Number(port)}`
}

export function parseDevArguments(args: string[]) {
    const remaining: string[] = []
    const environment: Record<string, string> = {}
    for (let index = 0; index < args.length; index++) {
        const argument = args[index]
        if (argument === undefined) break
        const name = argument.split('=')[0]
        if (name === '--backend-host' || name === '--backend-port') {
            const value = argument.includes('=')
                ? argument.slice(argument.indexOf('=') + 1)
                : args[++index]
            if (!value || value.startsWith('--')) {
                throw new Error(`${name} requires a value.`)
            }
            const key =
                name === '--backend-host'
                    ? 'DEV_BACKEND_HOST'
                    : 'DEV_BACKEND_PORT'
            if (key in environment)
                throw new Error(`${name} must only be provided once.`)
            environment[key] = value
        } else if (argument === '--no-open') {
            environment.OPEN_BROWSER = 'false'
        } else {
            remaining.push(argument)
        }
    }
    backendTarget(environment.DEV_BACKEND_HOST, environment.DEV_BACKEND_PORT)
    return { args: remaining, environment }
}

export function developmentOptions(
    command: string,
    environment: NodeJS.ProcessEnv,
) {
    if (command !== 'dev') return { open: false, target: undefined }
    const ci = Boolean(environment.CI) && environment.CI !== 'false'
    return {
        open: !ci && environment.OPEN_BROWSER !== 'false',
        target: backendTarget(
            environment.DEV_BACKEND_HOST,
            environment.DEV_BACKEND_PORT,
        ),
    }
}
