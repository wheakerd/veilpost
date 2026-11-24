import type { AppearanceMessages } from '../zh-CN/appearance'

export default {
    description:
        'Colors, vision mode, glass, page motion, and layout. Changes apply at once.',
    palette: {
        title: 'Theme color',
    },
    palettes: {
        abyss: { name: 'Deep Sea' },
        sky: { name: 'Clear Sky' },
        emerald: { name: 'Emerald' },
        mist: { name: 'Sea Mist' },
        amber: { name: 'Amber' },
        coral: { name: 'Coral' },
        rose: { name: 'Rose' },
        violet: { name: 'Ultraviolet' },
    },
    mode: {
        title: 'Appearance',
        light: 'Light',
        dark: 'Dark',
        system: 'System',
    },
    vision: {
        title: 'Vision mode',
        standard: 'Standard',
        colorSafe: 'Color-safe',
        grayscale: 'Grayscale',
        hints: {
            standard: 'Uses the theme color and the standard status colors',
            colorSafe:
                'Status colors switch to blue and vermilion, which stay apart with red–green color blindness',
            grayscale:
                'Removes every hue; brightness and shape tell states apart',
        },
    },
    glass: {
        title: 'Liquid glass',
        tint: 'Tint',
        tintMin: 'Clear',
        tintMax: 'Tinted',
        strength: 'Strength',
        strengthMin: 'Thin',
        strengthMax: 'Thick',
        description:
            'Adjusts the glass of drawers, dialogs, menus, and the top bar: more tint carries more theme color, more strength is clearer and blurrier. With reduced transparency on in your system, surfaces turn solid.',
        reset: 'Restore defaults',
    },
    transition: {
        title: 'Page transition',
        fade: 'Soft fade',
        slide: 'Slide in',
        zoom: 'Zoom focus',
        rise: 'Rise',
        description:
            'Hover or focus a card to preview it; the selected one plays when pages change. Nothing plays with reduced motion on.',
    },
    layout: {
        title: 'Layout and behavior',
        sidebarWidth: {
            atDefault: 'Already at the default width',
            title: 'Sidebar width',
            auto: 'Default {width} rem. Drag the sidebar edge to resize; double-click the edge to restore.',
            fixed: 'Fixed at {width} rem. Restoring the default lets it follow the window width.',
            reset: 'Restore default',
            handle: 'Resize sidebar',
        },
        navMultiple: {
            title: 'Sidebar groups',
            description:
                'On: several groups can be open at once. Off: opening a group closes the others.',
        },
        tabBar: {
            title: 'Tab bar',
            description:
                'Shows tabs for open pages; without it, switch pages from the sidebar',
        },
        progressBar: {
            title: 'Loading bar',
            description:
                'Shows a bar at the top of the screen while pages load, change, or refresh',
        },
        pageCache: {
            title: 'Keep page state',
            description:
                'Open pages keep their state and scroll position when you return; passwords and keys are still cleared when you leave. The refresh button reloads the current page.',
        },
        reducedMotion: {
            title: 'Reduce motion',
            description:
                'Minimizes movement of pages and controls while keeping status feedback. Also follows your system setting.',
        },
    },
    language: {
        title: 'Language',
        description: 'Follows your browser language by default',
        auto: 'Browser default ({language})',
    },
    footer: {
        note: 'Preferences are saved only in this browser',
        reset: 'Restore all defaults',
    },
} satisfies AppearanceMessages
