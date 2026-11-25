import type { CertsMessages } from '../zh-CN/certs'

export default {
    certificates: {
        description:
            'Request TLS certificates over ACME with DNS-01 validation; they renew automatically before they expire',
        request: 'Request certificate',
        listTitle: 'Certificates',
        listLabel: 'Certificates',
        columns: { domains: 'Domains', status: 'Status', actions: 'Actions' },
        count: '{count} certificate | {count} certificates',
        form: {
            description:
                'Issuance runs in the background; after you submit, the list shows “Issuing”. First add credentials under “DNS providers” that can manage these domains’ records.',
            domains: 'Domains',
            domainsHint:
                'One per line, or separated by commas or spaces. Wildcards like *.example.com are supported, up to 100 names.',
            credential: 'DNS provider credentials',
            credentialPlaceholder: 'Choose credentials',
            noCredentials: 'There are no DNS provider credentials yet',
            addCredential: 'Add credentials',
            directory: 'ACME service',
            directoryUrl: 'ACME directory URL',
            directoryUrlHint:
                'Only https:// URLs are accepted, such as the ZeroSSL or Google Trust Services directory',
            eabTitle: 'External account binding (EAB, optional)',
            eabHint:
                'Fill in when the CA requires it. It is used only to register the ACME account and is not kept afterwards.',
            eabKeyId: 'EAB key ID',
            eabHmacKey: 'EAB HMAC key',
            email: 'Contact email (optional)',
            emailHint:
                'Used as the ACME account contact. One account is shared per ACME service and email.',
            terms: 'By submitting, you agree to the terms of service of the selected ACME service',
            submit: 'Submit request',
            missingDomains: 'Enter at least one domain.',
            missingCredential: 'Choose DNS provider credentials.',
            missingDirectoryUrl:
                'Enter an ACME directory URL that starts with https://.',
            incompleteEab: 'Enter both the EAB key ID and the HMAC key.',
        },
        directories: {
            letsencrypt: "Let's Encrypt",
            letsencryptStaging: "Let's Encrypt staging",
            custom: 'Custom ACME directory',
        },
        state: {
            pending: 'Issuing',
            valid: 'Valid',
            failed: 'Failed',
            expired: 'Expired',
            renewing: 'Renewing',
        },
        meta: {
            issuer: 'Issuer',
            notAfter: 'Expires',
            renewAt: 'Renewal planned',
            directory: 'ACME service',
            credential: 'DNS credentials',
            email: 'Contact email',
            none: '—',
        },
        moreDomains: '{count} name in total | {count} names in total',
        lastError: 'Last error ({time}): {message}',
        issueErrors: {
            CREDENTIAL_UNAVAILABLE:
                'The DNS provider credentials are unavailable. Check or re-enter them.',
            ACME_UNREACHABLE:
                'The ACME service could not be reached. Check the directory URL and the network.',
            ACME_ACCOUNT_FAILED:
                'The ACME account could not be registered. If the CA requires EAB, enter it and request again.',
            ACME_RATE_LIMITED:
                'The ACME service hit a rate limit. The request is retried automatically later.',
            ACME_REJECTED:
                'The ACME service refused to issue. Check the domains and their DNS records.',
            DNS_PROVIDER_FAILED:
                'The validation record could not be created through the DNS provider. Make sure the credentials may edit these domains’ records.',
            DNS_PROPAGATION_TIMEOUT:
                'The validation record did not appear on the authoritative DNS servers in time.',
            ISSUANCE_FAILED: 'Issuance did not finish. Reissue later.',
        },
        reissue: 'Reissue',
        download: 'Download chain',
        delete: {
            open: 'Delete',
            description:
                'The certificate and its private key are removed from the console and cannot be restored; a running issuance stops',
            submit: 'Delete',
            done: 'The certificate was deleted',
        },
        requested: 'Request submitted; issuing in the background',
        reissued: 'Reissue started',
        downloaded:
            'Certificate chain downloaded. Private keys cannot be downloaded in the browser.',
        unavailable: {
            stale: 'Certificate status can’t be refreshed right now. Showing the last result.',
            empty: 'The certificate list can’t be read right now. Refresh the page later.',
        },
        empty: {
            title: 'No certificates yet',
            first: 'After you request a certificate, its status, issuer and expiry appear here',
            second: 'The console renews certificates 30 days before they expire',
        },
        note: 'Private keys are encrypted with the console master key and cannot be downloaded in the browser; node agents will fetch them when needed',
    },
    dns: {
        description:
            'Store provider credentials for DNS-01 validation. Secrets are write-only; the console only shows whether they are set.',
        add: 'Add credentials',
        listTitle: 'Credentials',
        listLabel: 'DNS provider credentials',
        count: '{count} credential | {count} credentials',
        providers: {
            cloudflare: 'Cloudflare',
            alidns: 'Alibaba Cloud DNS',
            tencentcloud: 'Tencent Cloud DNSPod',
            route53: 'AWS Route 53',
        },
        fields: {
            apiToken: 'API token',
            accessKeyId: 'Access key ID',
            accessKeySecret: 'AccessKey secret',
            secretId: 'SecretId',
            secretKey: 'SecretKey',
            secretAccessKey: 'Secret access key',
            region: 'Region',
            hostedZoneId: 'Hosted zone ID (optional)',
        },
        hints: {
            cloudflare:
                'An API token with the Zone:Read and DNS:Edit permissions',
            alidns: 'Prefer a RAM user AccessKey limited to Alibaba Cloud DNS',
            tencentcloud: 'Prefer a sub-account key limited to DNSPod',
            route53:
                'Needs permission to read Route 53 and change records; the region is usually us-east-1',
        },
        form: {
            createTitle: 'Add DNS provider credentials',
            editTitle: 'Edit credentials “{name}”',
            description:
                'Secrets are not shown again after saving. Enter your current password to confirm.',
            editDescription:
                'Secrets left empty keep their value. Enter your current password to confirm.',
            name: 'Note (optional)',
            nameHint:
                'Up to 64 characters; leave empty to display the provider name.',
            provider: 'Provider',
            keepValue: 'Set; leave empty to keep',
            submitCreate: 'Save credentials',
            submitEdit: 'Save changes',
            missing: 'Enter every required secret and your current password.',
            missingPassword: 'Enter your current password.',
            created: 'Credentials “{name}” saved',
            updated: 'Credentials “{name}” updated',
        },
        status: {
            set: 'Set',
            unset: 'Not set',
        },
        meta: {
            updatedAt: 'Last changed',
            usage: 'Usage',
            unused: 'Not used by certificates',
            usedBy: 'Used by {count} certificate | Used by {count} certificates',
        },
        verify: {
            open: 'Verify',
            done: 'Credentials “{name}” work: the provider accepted a read-only query',
        },
        edit: 'Edit',
        delete: {
            open: 'Delete',
            description:
                'This cannot be undone. Credentials used by a certificate cannot be deleted. Enter your current password to confirm.',
            submit: 'Delete',
            done: 'Credentials “{name}” were deleted',
        },
        unavailable: {
            stale: 'The credential list can’t be refreshed right now. Showing the last result.',
            empty: 'The credential list can’t be read right now. Refresh the page later.',
        },
        empty: {
            title: 'No DNS provider credentials yet',
            first: 'Cloudflare, Alibaba Cloud DNS, Tencent Cloud DNSPod and AWS Route 53 are supported',
            second: 'Add credentials to request certificates. Verifying only runs a read-only query and creates no records.',
        },
        note: 'Credentials are encrypted with the console master key; neither the API nor the logs return them',
    },
} satisfies CertsMessages
