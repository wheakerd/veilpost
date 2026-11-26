import type { OverviewMessages } from '../zh-CN/overview'

export default {
    description: 'Текущее состояние узлов, программ и сертификатов',
    viewNodes: 'Открыть узлы VPN',
    destinations: 'Управление',
    summary: 'Сводка',
    online: 'в сети: {online} из {total}',
    ofTotal: '{count} из {total}',
    stats: {
        nodes: 'Узлы в сети',
        nodesDetail: 'ожидают регистрации: {pending}',
        apps: 'Установленные программы',
        appsDetail: 'VPN-программы из встроенного каталога',
        certificates: 'Действующие сертификаты',
        nextExpiry: 'Ближайший срок {date}',
        noExpiry: 'Нет сертификатов с близким сроком',
        twoFactor: 'Двухэтапная проверка',
        enabled: 'Включена',
        disabled: 'Выключена',
        manage: 'Настроить',
    },
    nodes: {
        title: 'Состояние узлов',
        all: 'Все узлы',
        name: 'Имя',
        status: 'Состояние',
        lastSeen: 'Последняя связь',
        states: {
            online: 'В сети',
            offline: 'Не в сети',
            pending: 'Ожидает регистрации',
        },
        empty: 'Узлов пока нет. Создайте узел и выполните команду регистрации на его сервере, чтобы увидеть его состояние здесь.',
        unavailable: 'Состояние узлов сейчас недоступно',
    },
    expiry: {
        title: 'Сертификаты с ближайшим сроком',
        all: 'Все сертификаты',
        empty: 'Действующих сертификатов пока нет',
        unavailable: 'Сертификаты сейчас недоступны',
    },
} satisfies OverviewMessages
