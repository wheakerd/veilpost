const appearance = {
    description: '配色、视觉模式、玻璃质感、页面动画与布局行为。修改立即生效。',
    palette: {
        title: '主题色',
        preview: '左侧为浅色，右侧为深色。长条是主色，短条是副色。',
    },
    // Keyed by palette id in src/theme/palettes.ts.
    palettes: {
        lab: { name: '实验室绿' },
        abyss: { name: '深海蓝' },
        sky: { name: '晴空蓝' },
        amber: { name: '琥珀橙' },
        rose: { name: '玫瑰红' },
        violet: { name: '电光紫' },
    },
    mode: {
        title: '界面外观',
        light: '浅色',
        dark: '深色',
        system: '跟随系统',
    },
    vision: {
        title: '视觉模式',
        standard: '标准',
        colorSafe: '色弱友好',
        grayscale: '灰阶模式',
        hints: {
            standard: '使用当前主题色与标准状态颜色',
            colorSafe: '状态颜色改用蓝色与朱红色，红绿色弱也能区分',
            grayscale: '去掉所有色相，只用明暗与形状区分状态',
        },
    },
    glass: {
        title: '液态玻璃',
        tint: '色调',
        tintMin: '清透',
        tintMax: '着色',
        strength: '强度',
        strengthMin: '轻薄',
        strengthMax: '厚重',
        description:
            '调整抽屉、弹窗、菜单和顶栏的玻璃：越着色越带主题色，强度越高越透、越模糊。系统开启“降低透明度”时改为实色。',
        reset: '恢复默认',
    },
    transition: {
        title: '页面切换动画',
        fade: '柔和淡入',
        slide: '侧向滑入',
        zoom: '缩放聚焦',
        rise: '向上滑动',
        description:
            '悬停或聚焦卡片可预览，选中后用于页面切换；开启“减少动态效果”时不播放。',
    },
    layout: {
        title: '布局与行为',
        sidebarWidth: {
            atDefault: '已是默认宽度',
            title: '侧栏宽度',
            // {width} is a number of rem.
            auto: '默认 {width} rem。拖动侧栏右边缘可调整，双击边缘恢复默认。',
            fixed: '已固定为 {width} rem。恢复默认后随窗口宽度自动调整。',
            reset: '恢复默认',
            handle: '调整侧栏宽度',
        },
        navMultiple: {
            title: '菜单栏行为',
            description:
                '开启时可同时展开多个分组；关闭时展开一个分组会收起其他分组。',
        },
        tabBar: {
            title: '启用页签栏',
            description: '显示已打开页面的页签；关闭后通过侧栏切换页面',
        },
        progressBar: {
            title: '加载进度条',
            description: '在页面加载、切换和刷新时显示屏幕顶部的进度条',
        },
        pageCache: {
            title: '页面状态缓存',
            description:
                '返回已打开的页面时保留页面状态和滚动位置；密码和密钥仍在离开页面时清除。顶栏的刷新按钮会重新加载当前页。',
        },
        reducedMotion: {
            title: '减少动态效果',
            description:
                '减少页面与控件的位移，保留状态反馈。也会遵循系统设置。',
        },
    },
    language: {
        title: '界面语言',
        description: '默认跟随浏览器语言',
        // {language} is the name of the language matched from the browser.
        auto: '跟随浏览器（{language}）',
    },
    footer: {
        note: '偏好设置仅保存在此浏览器',
        reset: '全部恢复默认',
    },
}

export type AppearanceMessages = typeof appearance
export default appearance
