/** Text to search in: lower case, «ё» as «е». */
export function normalize(text: string): string {
  return text.toLocaleLowerCase().replaceAll('ё', 'е')
}

/**
 * How well a query fits a title and the other words of an item: null when a word of the query is in neither, 0 when
 * the title starts with the query, 1 when a word of the title does, 2 otherwise.
 */
export function score(query: string, title: string, extra = ''): number | null {
  const words = normalize(query).split(/\s+/).filter(Boolean)
  const name = normalize(title)
  const hay = `${name} ${normalize(extra)}`
  if (!words.every((w) => hay.includes(w))) return null
  const whole = words.join(' ')
  if (name.startsWith(whole)) return 0
  return name.split(/[\s·,—-]+/).some((part) => part.startsWith(words[0]!)) ? 1 : 2
}

/** The settings strings worth finding: short labels, not hints, texts or buttons. */
export function settingLabels(messages: unknown): string[] {
  const labels: string[] = []
  const walk = (value: unknown, key: string) => {
    if (typeof value === 'string') {
      if (!/hint|text|lead|note|title|placeholder|button|error|empty|ask|desc|failed|saved|copied/i.test(key) && value.length <= 48 && !/[{|]/.test(value)) {
        labels.push(value)
      }
    } else if (value && typeof value === 'object') {
      for (const [k, v] of Object.entries(value)) walk(v, k)
    }
  }
  walk(messages, '')
  return [...new Set(labels)]
}
