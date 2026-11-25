import type { ErrorMessages } from '../zh-CN/errors'

export default {
    INVALID_CREDENTIALS: 'Incorrect username or password. Try again.',
    INVALID_PASSWORD: 'Your current password is incorrect. Try again.',
    WEAK_PASSWORD:
        'The new password must be at least 12 characters and no more than 256 bytes.',
    PASSWORD_REUSED:
        'The new password must be different from your current password.',
    INVALID_CODE:
        'This code is incorrect, already used, or expired. Wait for the next code in your authenticator app.',
    INVALID_RECOVERY_CODE:
        'This recovery code is invalid or already used. Try another one.',
    CHALLENGE_EXPIRED: 'This verification has expired. Sign in again.',
    SETUP_EXPIRED: 'This setup has expired. Start again.',
    SESSION_EXPIRED: 'Your session has expired. Sign in again.',
    TOO_MANY_ATTEMPTS: 'Too many attempts. Try again later.',
    SERVICE_UNAVAILABLE:
        'The authentication service is unavailable. Try again later.',
    REQUEST_FAILED: 'The action didn’t complete. Try again.',
    INVALID_NAME:
        'Node names must be 1–64 characters with no line breaks or other control characters.',
    NODE_NOT_FOUND:
        'This node no longer exists. Refresh the list and try again.',
    NODE_NOT_PENDING:
        'This node is already enrolled or revoked, so its enrollment token can’t be reissued.',
    NODE_NOT_REVOKED:
        'Only revoked nodes can be deleted. Revoke the node first.',
    DOCKER_SOURCE_UNAVAILABLE:
        'The selected Docker package source is unavailable on this node.',
    DOCKER_INSTALL_BUSY: 'This node already has a Docker installation job.',
    APP_NOT_FOUND: 'This software isn’t in the catalog.',
    APP_NOT_INSTALLED:
        'This software isn’t installed. Install it from the store first.',
    APP_ALREADY_INSTALLED: 'This software is already installed.',
    APP_DEPLOYED:
        'This software is deployed to nodes. Remove it from every node before uninstalling.',
    INVALID_SETTING: 'A setting has an invalid value. Check it and try again.',
    SETTING_REQUIRED: 'Some required settings are empty.',
    UNKNOWN_SETTING:
        'The settings include fields this software doesn’t support. Refresh the page and try again.',
    INVALID_CREDENTIAL_NAME:
        'Credential names need 1–64 characters without line breaks or other control characters.',
    INVALID_PROVIDER: 'This DNS provider is not supported.',
    INVALID_SECRETS:
        'Some credential fields are missing or malformed. Check them and try again.',
    CREDENTIAL_NOT_FOUND:
        'These DNS provider credentials no longer exist. Refresh the list and try again.',
    CREDENTIAL_IN_USE:
        'These credentials are used by a certificate. Delete those certificates first.',
    CREDENTIAL_UNREADABLE:
        'The stored credentials can’t be read. Enter every secret again.',
    CREDENTIAL_REJECTED:
        'The provider rejected these credentials. Check the keys and their permissions.',
    PROVIDER_UNREACHABLE:
        'The DNS provider can’t be reached right now. Try again later.',
    INVALID_DOMAIN:
        'Invalid domain: only valid DNS names are accepted, and wildcards must look like *.example.com.',
    TOO_MANY_DOMAINS: 'A certificate can hold at most 100 names.',
    INVALID_DIRECTORY:
        'Invalid ACME directory URL. Only https:// URLs are accepted.',
    INVALID_EMAIL: 'The contact email is not valid.',
    INVALID_EAB:
        'Invalid EAB: it is only available with a custom ACME directory, and the HMAC key must be base64url encoded.',
    CERTIFICATE_NOT_FOUND:
        'The certificate no longer exists. Refresh the list and try again.',
    CERTIFICATE_NOT_ISSUED:
        'The certificate has not been issued yet, so it can’t be downloaded.',
} satisfies ErrorMessages
