import { createI18n } from 'vue-i18n'

import en from './en.json'
import { resolveLocale } from './locale'
import ru from './ru.json'

export function createAppI18n(languages: readonly string[]) {
  return createI18n({
    legacy: false,
    locale: resolveLocale(languages),
    fallbackLocale: 'en',
    messages: { en, ru },
  })
}
