// How the screens write numbers, sizes, times and countries, in the interface's language.

const units = ['B', 'KB', 'MB', 'GB', 'TB'] as const

/** A size in bytes in the largest unit that keeps it above one: 38,2 ГБ. */
export function formatBytes(bytes: number, locale: string, unit: (name: string) => string): string {
  let value = Math.max(0, bytes)
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index++
  }
  const digits = index === 0 || value >= 100 ? 0 : 1
  const number = new Intl.NumberFormat(locale, { maximumFractionDigits: digits }).format(value)
  return `${number} ${unit(units[index]!)}`
}

/** Hours, minutes and seconds since a moment: 00:12:48. */
export function formatDuration(milliseconds: number): string {
  const total = Math.max(0, Math.floor(milliseconds / 1000))
  const parts = [Math.floor(total / 3600), Math.floor((total % 3600) / 60), total % 60]
  return parts.map((part) => String(part).padStart(2, '0')).join(':')
}

/** How long ago a moment was: «2 часа назад», «вчера». */
export function formatAgo(when: number, now: number, locale: string): string {
  const format = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
  const minutes = (when - now) / 60_000
  if (Math.abs(minutes) < 1) return format.format(0, 'minute')
  if (Math.abs(minutes) < 60) return format.format(Math.round(minutes), 'minute')
  const hours = minutes / 60
  if (Math.abs(hours) < 24) return format.format(Math.round(hours), 'hour')
  const days = hours / 24
  if (Math.abs(days) < 30) return format.format(Math.round(days), 'day')
  const months = days / 30
  if (Math.abs(months) < 12) return format.format(Math.round(months), 'month')
  return format.format(Math.round(months / 12), 'year')
}

/** A date without the time: 12 декабря, with the year when it is not this one (12 декабря 2027 г.). */
export function formatDate(when: number, locale: string, now = Date.now()): string {
  const year = new Date(when).getFullYear() !== new Date(now).getFullYear() ? 'numeric' : undefined
  return new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'long', year }).format(when)
}

/** The country of an ISO 3166 code in the interface's language; the code itself when the system has no name. */
export function countryName(code: string, locale: string): string {
  if (!code) return ''
  try {
    return new Intl.DisplayNames([locale], { type: 'region' }).of(code.toUpperCase()) ?? code
  } catch {
    return code
  }
}

/** The flag of an ISO 3166 code, as its two regional indicator letters; "" for anything else. */
export function flagOf(code: string): string {
  const upper = code.toUpperCase()
  if (!/^[A-Z]{2}$/.test(upper)) return ''
  return String.fromCodePoint(...[...upper].map((letter) => 0x1f1e6 + letter.charCodeAt(0) - 65))
}

/** A country for a hint beside its flag: «DE — Германия», the code alone when the system has no name. */
export function countryHint(code: string, locale: string): string {
  const upper = code.toUpperCase()
  const name = countryName(upper, locale)
  return name && name !== upper ? `${upper} — ${name}` : upper
}

/** The colour of a delay: good under 80 ms, middling under 180, bad above, none when untested or silent. */
export function latencyTone(latency: number): 'good' | 'mid' | 'bad' | 'none' {
  if (latency <= 0) return 'none'
  if (latency < 80) return 'good'
  if (latency < 180) return 'mid'
  return 'bad'
}

const leadingFlag = /^(?:\p{Regional_Indicator}{2}\s*)+/u

/** A server's name without the flag it starts with, when its country is shown beside it anyway. */
export function serverTitle(name: string, country: string): string {
  if (!country) return name
  const stripped = name.replace(leadingFlag, '').trim()
  return stripped || name
}
