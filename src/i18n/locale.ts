export type Locale = 'en' | 'ru'

/** The first supported language of the system list wins, English otherwise. */
export function resolveLocale(languages: readonly string[]): Locale {
  for (const tag of languages) {
    const base = tag.toLowerCase().split('-')[0]
    if (base === 'ru' || base === 'en') return base
  }
  return 'en'
}
