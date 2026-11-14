import { describe, expect, it } from 'vitest'

// The code of the interface without its tests: Russian words there would bypass the translation in en.json and ru.json.
const sources = import.meta.glob<string>(['/src/**/*.{vue,ts}', '!/src/**/*.test.ts'], {
  query: '?raw',
  import: 'default',
  eager: true,
})

/** Russian words that belong in the code, by file, with the reason. */
const allowed: Record<string, { words: string[]; why: string }> = {
  '/src/components/settings/LookPage.vue': { words: ['Русский'], why: 'each language is named in its own words' },
  '/src/lib/palette.ts': { words: ['ё', 'е'], why: 'the search folds «ё» into «е», as Russian text is typed' },
}

// Comments may quote the screens in Russian: line comments, blocks and comments of the templates go first.
const withoutComments = (code: string) =>
  code
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/(^|\s)\/\/.*$/gm, '$1')

describe('the code of the interface', () => {
  it('is read, with the words of the allowed list still in it', () => {
    expect(Object.keys(sources)).toContain('/src/App.vue')
    for (const [file, { words }] of Object.entries(allowed)) {
      for (const word of words) expect(withoutComments(sources[file] ?? ''), file).toContain(word)
    }
  })

  it('has no Russian words outside the messages', () => {
    const found = Object.entries(sources).flatMap(([file, code]) => {
      const words = withoutComments(code).match(/[\u0400-\u04ff]+/g) ?? []
      const extra = words.filter((word) => !allowed[file]?.words.includes(word))
      return extra.length > 0 ? [`${file}: ${[...new Set(extra)].join(', ')}`] : []
    })
    expect(found).toEqual([])
  })
})
