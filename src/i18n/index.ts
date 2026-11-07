import { createI18n } from 'vue-i18n'

import en from './en.json'
import { resolveLocale } from './locale'
import ru from './ru.json'

/** Russian forms in the messages: zero | one | few | many, as in "0 серверов | 1 сервер | 2 сервера | 5 серверов". */
export function russianPlural(choice: number, choicesLength: number): number {
  if (choicesLength < 4) return choice === 0 ? 0 : choice === 1 ? 1 : 2
  if (choice === 0) return 0
  const tens = choice % 100
  const ones = choice % 10
  if (tens >= 11 && tens <= 14) return 3
  if (ones === 1) return 1
  if (ones >= 2 && ones <= 4) return 2
  return 3
}

export function createAppI18n(languages: readonly string[]) {
  return createI18n({
    legacy: false,
    locale: resolveLocale(languages),
    fallbackLocale: 'en',
    messages: { en, ru },
    pluralRules: { ru: russianPlural },
  })
}
