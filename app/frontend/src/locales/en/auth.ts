import type { AuthMessages } from '../zh-CN/auth'

export default {
    layout: {
        meta: 'Local console',
        status: 'Awaiting sign-in',
        statusVerify: 'Awaiting two-step verification',
        footer: 'Authorized administrators only',
        entering: 'Entering the console…',
    },
    login: {
        step: '01 · Account and password',
        // {key} is rendered as a keycap.
        keyHint: 'Press {key} to sign in',
        title: 'Sign in to the console',
        description: 'Continue with your administrator account',
        expired: 'Your sign-in or verification has expired. Sign in again.',
        retry: 'Retry connection',
        setupRequired:
            'No administrator exists yet. Run {command} in a terminal in the project directory on this machine to create one.',
        recheck: 'I’ve created an account, check again',
        username: 'Username',
        missingCredentials: 'Enter your username and password.',
        connecting: 'Connecting…',
        signingIn: 'Signing in…',
        submit: 'Sign in',
        helpTwoFactor:
            'If two-step verification is on, you’ll confirm your identity in the next step',
        helpContact: 'Can’t sign in? Contact your console administrator',
    },
    verify: {
        step: '02 · One-time code',
        title: 'Two-step verification',
        totpDescription:
            'Open your authenticator app and enter the current 6-digit code',
        recoveryDescription:
            'Enter a recovery code you haven’t used yet. Each code works only once.',
        remaining: 'Time left to verify: {time}',
        recoveryLabel: 'Recovery code',
        totpHelp:
            'Codes refresh every 30 seconds, and each code can be used only once',
        recoveryHelp:
            'Recovery codes are the backup codes you saved when you turned on two-step verification',
        recoveryRequired: 'Enter a recovery code.',
        passed: 'Verified, continuing…',
        verifying: 'Verifying…',
        submit: 'Verify and sign in',
        useRecovery: 'Use a recovery code',
        useAuthenticator: 'Use authenticator app',
        back: 'Back to sign-in',
    },
} satisfies AuthMessages
