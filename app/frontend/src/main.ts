import { createApp } from 'vue'
import { createPinia } from 'pinia'
import piniaPluginPersistedstate from 'pinia-plugin-persistedstate'
import App from './App.vue'
import { router } from './router'
import { usePreferencesStore } from './stores/preferences'
import { installTheme } from './theme'
import { i18n, installLocale } from './i18n'
import { migrateLegacyPreferences } from './theme/palettes'
import { installGlassLight, installSheetLens } from './theme/liquidGlass'
// Component styles arrive with each component (unplugin-vue-components);
// these are the shared variables and the dark-mode overrides.
import 'element-plus/theme-chalk/base.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import './styles/index.scss'
import './styles/_auth.scss'
import './styles/_apps.scss'
import './styles/_preferences.scss'
import './styles/_glass.scss'
import './styles/_type.scss'
import './styles/_palettes.scss'
import './styles/_theme-transition.scss'

migrateLegacyPreferences(() => window.localStorage)
const app = createApp(App)
const pinia = createPinia()
pinia.use(piniaPluginPersistedstate)
app.use(pinia)
const preferences = usePreferencesStore(pinia)
installTheme(preferences)
installLocale(preferences)
app.use(i18n)
app.use(router)
installGlassLight()
installSheetLens()
app.mount('#root')
