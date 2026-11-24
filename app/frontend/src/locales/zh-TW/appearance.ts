import type { AppearanceMessages } from '../zh-CN/appearance'

export default {
    description: '配色、視覺模式、玻璃質感、頁面動畫與版面行為。變更立即生效。',
    palette: {
        title: '主題色',
    },
    palettes: {
        abyss: { name: '深海藍' },
        sky: { name: '晴空藍' },
        emerald: { name: '翡翠綠' },
        mist: { name: '霧海青' },
        amber: { name: '琥珀橙' },
        coral: { name: '珊瑚橙' },
        rose: { name: '玫瑰紅' },
        violet: { name: '電光紫' },
    },
    mode: {
        title: '介面外觀',
        light: '淺色',
        dark: '深色',
        system: '跟隨系統',
    },
    vision: {
        title: '視覺模式',
        standard: '標準',
        colorSafe: '色弱友善',
        grayscale: '灰階模式',
        hints: {
            standard: '使用目前的主題色與標準狀態顏色',
            colorSafe: '狀態顏色改用藍色與朱紅色，紅綠色弱也能分辨',
            grayscale: '移除所有色相，只用明暗與形狀區分狀態',
        },
    },
    glass: {
        title: '液態玻璃',
        tint: '色調',
        tintMin: '清透',
        tintMax: '著色',
        strength: '強度',
        strengthMin: '輕薄',
        strengthMax: '厚重',
        description:
            '調整抽屜、對話框、選單和頂欄的玻璃：越著色越帶主題色，強度越高越透、越模糊。系統開啟「降低透明度」時改為實色。',
        reset: '還原預設值',
    },
    transition: {
        title: '頁面切換動畫',
        fade: '柔和淡入',
        slide: '側向滑入',
        zoom: '縮放聚焦',
        rise: '向上滑動',
        description:
            '懸停或聚焦卡片即可預覽，選取後用於頁面切換；開啟「減少動態效果」時不播放。',
    },
    layout: {
        title: '版面與行為',
        sidebarWidth: {
            atDefault: '已是預設寬度',
            title: '側欄寬度',
            auto: '預設 {width} rem。拖曳側欄右緣可調整，按兩下邊緣還原預設值。',
            fixed: '已固定為 {width} rem。還原預設值後隨視窗寬度自動調整。',
            reset: '還原預設值',
            handle: '調整側欄寬度',
        },
        navMultiple: {
            title: '選單列行為',
            description:
                '開啟時可同時展開多個群組；關閉時展開一個群組會收合其他群組。',
        },
        tabBar: {
            title: '啟用頁籤列',
            description: '顯示已開啟頁面的頁籤；關閉後透過側欄切換頁面',
        },
        progressBar: {
            title: '載入進度條',
            description: '在頁面載入、切換和重新整理時顯示畫面頂端的進度條',
        },
        pageCache: {
            title: '頁面狀態快取',
            description:
                '回到已開啟的頁面時保留頁面狀態和捲動位置；密碼與金鑰仍會在離開頁面時清除。頂欄的重新整理按鈕會重新載入目前頁面。',
        },
        reducedMotion: {
            title: '減少動態效果',
            description:
                '減少頁面與控制項的位移，保留狀態回饋。也會遵循系統設定。',
        },
    },
    language: {
        title: '介面語言',
        description: '預設跟隨瀏覽器語言',
        auto: '跟隨瀏覽器（{language}）',
    },
    footer: {
        note: '偏好設定只儲存在此瀏覽器',
        reset: '全部還原預設值',
    },
} satisfies AppearanceMessages
