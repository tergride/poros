import { describe, expect, it } from 'vitest'

import { shellReasons } from '@/composables/useReason'
import * as coreTypes from '@/platform/core-types'

import en from './en.json'
import { createAppI18n } from './index'
import ru from './ru.json'

type Messages = string | Messages[] | { [key: string]: Messages }
type Locale = 'en' | 'ru'

/**
 * Every leaf of the messages by its path, as vue-i18n reads it: `a.b` for objects, `a[0]` for the items of a list.
 * A list itself is its length.
 */
function leaves(node: Messages, path = '', out = new Map<string, string | number>()) {
  if (typeof node === 'string') out.set(path, node)
  else if (Array.isArray(node)) {
    out.set(path, node.length)
    node.forEach((item, i) => leaves(item, `${path}[${i}]`, out))
  } else for (const [key, value] of Object.entries(node)) leaves(value, path ? `${path}.${key}` : key, out)
  return out
}

const messages: Record<Locale, Map<string, string | number>> = { en: leaves(en), ru: leaves(ru) }
const strings = (locale: Locale) =>
  [...messages[locale]].filter((entry): entry is [string, string] => typeof entry[1] === 'string')
const text = (locale: Locale, key: string) => String(messages[locale].get(key) ?? '')

// A literal, {'…'}, is text that only looks like a placeholder: it is compared as it is and never filled.
const literal = /\{'[^']*'\}/g
const placeholders = (message: string) =>
  [...new Set(message.match(/\{'[^']*'\}|\{\s*[\w$]+\s*\}/g)?.map((p) => p.replace(/\s/g, '')))].sort()
const forms = (message: string) => message.replace(literal, '').split('|')

describe('the messages of the interface', () => {
  it('have the same keys and lists in English and Russian', () => {
    const missing = (from: Locale, to: Locale) => [...messages[from].keys()].filter((key) => !messages[to].has(key))
    expect(missing('en', 'ru'), 'keys only in en.json').toEqual([])
    expect(missing('ru', 'en'), 'keys only in ru.json').toEqual([])
    const lists = (locale: Locale) => [...messages[locale]].filter(([, value]) => typeof value === 'number')
    expect(lists('ru')).toEqual(lists('en'))
  })

  it('have the same placeholders in both languages', () => {
    const differ = strings('en')
      .filter(([key, message]) => placeholders(message).join() !== placeholders(text('ru', key)).join())
      .map(([key, message]) => `${key}: ${placeholders(message).join(' ')} / ${placeholders(text('ru', key)).join(' ')}`)
    expect(differ).toEqual([])
  })

  // English goes by the rule of vue-i18n: "one | other", or "zero | one | other". Russian goes by russianPlural, which
  // needs "zero | one | few | many": with fewer forms one form would stand for both «2 сервера» and «5 серверов».
  it('keep their plural forms: two or three in English, four in Russian', () => {
    const wrong = strings('en').flatMap(([key, message]) => {
      const english = forms(message).length
      const russian = forms(text('ru', key)).length
      if (english === 1 && russian === 1) return []
      if ((english === 2 || english === 3) && russian === 4) return []
      return [`${key}: ${english} forms in English, ${russian} in Russian`]
    })
    expect(wrong).toEqual([])
    const empty = (['en', 'ru'] as const).flatMap((locale) =>
      strings(locale)
        .filter(([, message]) => forms(message).some((form) => form.trim() === ''))
        .map(([key]) => `${locale} ${key}`),
    )
    expect(empty).toEqual([])
  })

  it('render in both languages with their placeholders and counts', () => {
    for (const locale of ['en', 'ru'] as const) {
      const { t } = createAppI18n([locale]).global
      for (const [key, message] of strings(locale)) {
        const values = Object.fromEntries(
          placeholders(message)
            .filter((p) => !p.startsWith("{'"))
            .map((p) => [p.slice(1, -1), 'x']),
        )
        const literals = (message.match(literal) ?? []).map((l) => l.slice(2, -2))
        const counts = forms(message).length > 1 ? [0, 1, 2, 5, 21] : [undefined]
        for (const n of counts) {
          const shown = n === undefined ? t(key, values) : t(key, { ...values, n }, n)
          expect(shown, `${locale} ${key}`).not.toBe(key)
          expect(shown.trim(), `${locale} ${key} ${n ?? ''}`).not.toBe('')
          const unfilled = literals.reduce((rest, l) => rest.replace(l, ''), shown)
          expect(unfilled, `${locale} ${key}`).not.toMatch(/\{\s*[\w$]+\s*\}/)
        }
      }
    }
  })
})

describe('the reasons of failures', () => {
  // The interface shows a reason in words; without them a failure shows the core's English text.
  const coreReasons = Object.entries(coreTypes)
    .filter(([name, value]) => name.startsWith('Reason') && typeof value === 'string')
    .map(([, value]) => value as string)
  const installReasons = [coreTypes.InstallBusy, coreTypes.InstallDamaged, coreTypes.InstallFailed]

  it('of the core are found among its constants', () => {
    expect(coreReasons).toContain(coreTypes.ReasonInvalidURL)
    expect(coreReasons.length).toBeGreaterThan(40)
  })

  it('of the core, the shell and the updater have words in both languages', () => {
    const reasons = [...coreReasons, ...Object.values(shellReasons)]
    for (const locale of ['en', 'ru'] as const) {
      expect(reasons.filter((reason) => typeof messages[locale].get(`reason.${reason}`) !== 'string'), locale).toEqual([])
      const updater = installReasons.filter((reason) => typeof messages[locale].get(`updates.failed.${reason}`) !== 'string')
      expect(updater, locale).toEqual([])
    }
  })
})
