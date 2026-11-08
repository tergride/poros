import { createPinia } from 'pinia'
import { createApp } from 'vue'

import App from './App.vue'
import { createAppI18n } from './i18n'
import { keepBrowserOut } from './lib/browserKeys'
import { logError } from './lib/logError'
import { router } from './router'
import './styles/main.css'

keepBrowserOut(window, import.meta.env.DEV)
const i18n = createAppI18n(navigator.languages)
document.documentElement.lang = i18n.global.locale.value

// What slips past the handlers still reaches the log, so that a broken screen leaves a trace.
window.addEventListener('error', (event) => logError('script error', event.error ?? event.message))
window.addEventListener('unhandledrejection', (event) => logError('unhandled error', event.reason))

const app = createApp(App)
app.config.errorHandler = (error) => {
  // A handler of its own keeps Vue from printing the error; the console still gets it.
  console.error(error)
  logError('interface error', error)
}
app.use(createPinia()).use(i18n).use(router).mount('#app')
