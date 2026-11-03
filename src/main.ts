import { createPinia } from 'pinia'
import { createApp } from 'vue'

import App from './App.vue'
import { createAppI18n } from './i18n'
import { router } from './router'
import './styles/main.css'

const i18n = createAppI18n(navigator.languages)
document.documentElement.lang = i18n.global.locale.value

createApp(App).use(createPinia()).use(i18n).use(router).mount('#app')
