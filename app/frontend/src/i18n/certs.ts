import type { DnsProvider } from '@/api/certs'
import { i18n } from '.'

/** An optional note, falling back to the provider in the interface language. */
export function credentialLabel(name: string, provider: DnsProvider) {
    return name.trim() || i18n.global.t(`certs.dns.providers.${provider}`)
}
