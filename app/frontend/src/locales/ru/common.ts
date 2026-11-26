import type { CommonMessages } from '../zh-CN/common'

export default {
    language: {
        self: 'Русский',
        // Short label on the sign-in language switch.
        short: 'RU',
    },
    app: {
        name: 'Veilpost',
        description: 'Единая панель управления собственными VPN-узлами',
        home: 'Veilpost — главная',
    },
    routes: {
        login: 'Вход',
        verify: 'Двухэтапная проверка',
        overview: 'Обзор сети',
        nodes: 'VPN-узлы',
        appearance: 'Настройки',
        security: 'Защита аккаунта',
        certificates: 'Сертификаты',
        dnsProviders: 'DNS-провайдеры',
        notFound: 'Страница не найдена',
        store: 'Магазин',
        installedApp: 'Установленные программы',
    },
    shell: {
        account: {
            title: 'Личный кабинет',
            open: 'Открыть личный кабинет',
            profile: 'Профиль',
            loginSecurity: 'Вход и безопасность',
            twoFactorOn: 'Двухэтапная проверка включена',
            twoFactorOff: 'Двухэтапная проверка выключена',
        },
        hideTabs: 'Скрыть панель вкладок',
        skipLink: 'Перейти к основному содержимому',
        navigation: 'Основная навигация',
        groups: {
            workspace: 'Рабочая область',
            software: 'Программы',
            security: 'Безопасность',
            settings: 'Настройки',
        },
        breadcrumb: 'Консоль',
        signOut: 'Выйти',
        installedApps: 'Установленные программы',
        collapse: 'Свернуть боковую панель',
        expand: 'Развернуть боковую панель',
        refresh: 'Обновить страницу',
        mode: 'Оформление: {mode}',
        modes: { light: 'Светлое', dark: 'Тёмное' },
        language: 'Язык интерфейса',
        followBrowser: 'Как в браузере',
        preferences: 'Настройки',
        clock: 'Местное время',
        version: 'v{version}',
        openPages: 'Открытые страницы',
        closeTab: 'Закрыть «{title}»',
        closeOthers: 'Закрыть остальные',
    },
    loading: {
        title: 'Загрузка консоли',
    },
    notFound: {
        title: 'Такой страницы нет',
        description: 'Проверьте адрес или вернитесь к обзору сети',
        back: 'К обзору сети',
    },
    status: {
        unavailable: 'Недоступно',
        loading: 'Загрузка',
        notConnected: 'Не подключены',
    },
    discard: {
        message: 'Отменить несохранённые изменения?',
        confirm: 'Отменить изменения',
        keep: 'Продолжить',
    },
    password: {
        label: 'Пароль',
        current: 'Текущий пароль',
        showLabel: 'Показать пароль',
        hideLabel: 'Скрыть пароль',
        capsLock: 'Включён Caps Lock',
    },
    code: {
        label: 'Код подтверждения',
        required: 'Введите 6-значный код подтверждения.',
    },
    actions: {
        cancel: 'Отмена',
        copied: 'Скопировано',
        copyFailed:
            'Не удалось скопировать автоматически. Выделите текст и скопируйте вручную.',
    },
    time: {
        justNow: 'только что',
    },
} satisfies CommonMessages
