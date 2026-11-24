import type { SecurityMessages } from '../zh-CN/security'

export default {
    description: 'Manage how you sign in and protect your network console',
    twoFactor: {
        title: 'Two-step verification',
        description: 'Confirm your identity once more, beyond your password',
        enabled: 'On',
        disabled: 'Off',
        protectedTitle: 'An authenticator app protects your account',
        unprotectedTitle: 'Protect sign-in with an authenticator app',
        protectedBody:
            'Enter the code from your authenticator app when you sign in. If you can’t use it, use a saved recovery code.',
        unprotectedBody:
            'Once you link an authenticator app, signing in also requires a 6-digit code. Works with common TOTP authenticator apps.',
        recoveryRemaining:
            '{count} recovery code left | {count} recovery codes left',
        recoveryLow:
            '{count} recovery code left. Consider generating new ones. | {count} recovery codes left. Consider generating new ones.',
        regenerate: 'Generate new recovery codes',
        enable: 'Turn on two-step verification',
        disable: 'Turn off two-step verification',
    },
    confirm: {
        title: 'First, confirm it’s you',
        description:
            'Enter your current password to start linking an authenticator app',
        submit: 'Continue',
    },
    setup: {
        scanTitle: 'Scan the QR code',
        scanBody:
            'In your authenticator app, choose “Add account” and scan the QR code below',
        qrAlt: 'QR code for linking an authenticator app',
        added: 'Once added, the app shows a 6-digit code that refreshes every 30 seconds',
        qrUnavailable:
            'The QR code can’t be shown right now. Add the account manually.',
        manualSummary: 'Can’t scan? Enter the key manually',
        manualKey: 'Choose “Time-based” as the type and enter this setup key:',
        copyKey: 'Copy key',
        verifyTitle: 'Verify and finish',
        verifyBody:
            'Enter the code shown for the new account. Two-step verification turns on only after this succeeds.',
        submit: 'Verify and turn on',
        cancel: 'Cancel setup',
        expired: 'This setup has expired. Start again.',
    },
    recovery: {
        confirmFirst: 'Confirm that you saved the recovery codes first',
        title: 'Save your recovery codes',
        body: 'These recovery codes are shown only here. Store them somewhere safe, such as a password manager. If you lose your authenticator app, each code lets you sign in once.',
        listLabel: 'Recovery codes',
        copy: 'Copy recovery codes',
        copied: 'Copied. Store them somewhere safe.',
        saved: 'I’ve saved my recovery codes somewhere safe',
        finishSetup: 'Finish setup',
        finish: 'Done',
        leaveTitle: 'Recovery codes not saved',
        leave: 'Leave anyway',
        stay: 'Stay on this page',
        leaveConfirm:
            'Recovery codes are shown only here. If you leave before saving them, you may not be able to get them back. Leave anyway?',
        regenerated:
            'New recovery codes are active. All old codes no longer work.',
        enabled:
            'Two-step verification is on. You’ll confirm your identity the next time you sign in.',
    },
    regenerate: {
        title: 'Generate new recovery codes',
        description:
            'All old recovery codes stop working immediately. Enter your current password and a code from your authenticator app to confirm.',
        submit: 'Generate new codes',
    },
    disable: {
        title: 'Turn off two-step verification',
        description:
            'Signing in will then require only your username and password, and existing recovery codes will stop working. Enter your current password and a code from your authenticator app to confirm.',
        submit: 'Turn off two-step verification',
        done: 'Two-step verification is off',
    },
    password: {
        title: 'Sign-in password',
        description:
            'Change your password regularly. After a change, other devices must sign in again.',
        change: 'Change password',
        next: 'New password',
        confirm: 'Confirm new password',
        hint: 'At least 12 characters, different from your current password',
        save: 'Save new password',
        missing: 'Enter your current password and a new password.',
        mismatch: 'The new passwords don’t match.',
        changed: 'Password changed. Other devices have been signed out.',
    },
    sessions: {
        noOthers: 'There are no other sessions to sign out',
        title: 'Sign-in sessions',
        unavailable:
            'Can’t load session information right now. Refresh the page later.',
        loading: 'Checking other sessions…',
        others: 'Besides this browser, {count} other session is signed in | Besides this browser, {count} other sessions are signed in',
        onlyCurrent: 'Only this browser is signed in',
        revoke: 'Sign out other sessions',
        revoked: 'Other sessions have been signed out',
    },
    lostEverything:
        'If both your authenticator app and recovery codes are lost, ask the server administrator to run {command} to restore access',
} satisfies SecurityMessages
