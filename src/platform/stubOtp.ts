import type { CoreError } from './core'
import type { OTPCode, OTPEntry, OTPImported, OTPMove, OTPParsed, OTPProblem } from './core-types'
import {
  OTPKindHOTP,
  OTPKindTOTP,
  OTPMaxDigits,
  OTPMaxPeriod,
  OTPMinDigits,
  OTPMinPeriod,
  OTPSHA1,
  OTPSHA256,
  OTPSHA512,
  ReasonInvalidOTP,
  ReasonNotFound,
} from './core-types'

// «Tools → OTP» of the stub core: the entries and their checks as core/internal/otp keeps them. The codes are not
// HMAC: a hash of the secret and the step stands in for them, which is all the interface can tell.

const base32 = /^[A-Z2-7]+$/

/** The canonical secret: upper case without spaces, dashes or padding; '' when it is not base32. */
export function normalizeSecret(secret: string): string {
  const s = secret.replace(/[\s\-_=]/g, '').toUpperCase()
  return base32.test(s) ? s : ''
}

function fakeCode(secret: string, step: number, digits: number): string {
  let hash = 2166136261
  for (const c of `${secret}:${step}`) hash = Math.imul(hash ^ c.charCodeAt(0), 16777619) >>> 0
  return String(hash % 10 ** Math.min(digits, 9)).padStart(digits, '0')
}

function problemOf(entry: OTPEntry): OTPProblem | null {
  const fail = (reason: string, value = ''): OTPProblem => ({ line: 0, name: entry.name, reason, value })
  if (!entry.secret.trim()) return fail('no_secret')
  if (entry.kind !== OTPKindTOTP && entry.kind !== OTPKindHOTP) return fail('bad_kind', entry.kind)
  if (![OTPSHA1, OTPSHA256, OTPSHA512].includes(entry.algorithm)) return fail('bad_algorithm', entry.algorithm)
  if (entry.digits < OTPMinDigits || entry.digits > OTPMaxDigits) return fail('bad_digits', String(entry.digits))
  if (entry.kind === OTPKindTOTP && (entry.period < OTPMinPeriod || entry.period > OTPMaxPeriod)) return fail('bad_period', String(entry.period))
  if (normalizeSecret(entry.secret) !== entry.secret) return fail('bad_secret')
  return null
}

const fresh = (secret: string): OTPEntry => ({ id: '', name: '', account: '', secret, kind: OTPKindTOTP, algorithm: OTPSHA1, digits: 6, period: 30, counter: 0 })

function parseLink(line: string): OTPEntry | OTPProblem {
  let u: URL
  try {
    u = new URL(line)
  } catch {
    return { line: 0, name: '', reason: 'bad_link', value: '' }
  }
  const entry = fresh('')
  const label = decodeURIComponent(u.pathname.replace(/^\/+/, ''))
  const [name, account] = label.includes(':') ? label.split(/:(.*)/s) : ['', label]
  entry.name = u.searchParams.get('issuer')?.trim() || name!.trim()
  entry.account = (account ?? '').trim()
  if (!entry.name) [entry.name, entry.account] = [entry.account, '']
  entry.kind = u.host.toLowerCase()
  const secret = u.searchParams.get('secret') ?? ''
  if (!secret.trim()) return { line: 0, name: entry.name, reason: 'no_secret', value: '' }
  entry.secret = normalizeSecret(secret)
  if (!entry.secret) return { line: 0, name: entry.name, reason: 'bad_secret', value: '' }
  if (u.searchParams.has('digits')) entry.digits = Number(u.searchParams.get('digits'))
  if (u.searchParams.has('period')) entry.period = Number(u.searchParams.get('period'))
  if (entry.kind === OTPKindHOTP) {
    entry.period = 0
    entry.counter = Number(u.searchParams.get('counter') ?? 0)
  }
  return problemOf(entry) ?? entry
}

// As core/internal/otp: one word, or groups of the same width, is a printed secret rather than prose.
function looksLikeSecret(line: string): boolean {
  const groups = line.split(/[\s-]+/).filter(Boolean)
  if (groups.length === 1) return true
  return groups.length > 1 && groups[0]!.length <= 8 && groups.every((g) => g.length === groups[0]!.length)
}

export function parseOtp(text: string): OTPParsed {
  const parsed: OTPParsed = { entries: [], problems: [] }
  text.split('\n').forEach((raw, i) => {
    const line = raw.trim()
    if (!line) return
    const at = (p: OTPProblem) => parsed.problems.push({ ...p, line: i + 1 })
    if (/^otpauth:\/\//i.test(line)) {
      const result = parseLink(line)
      if ('reason' in result) at(result)
      else parsed.entries.push(result)
    } else if (looksLikeSecret(line)) {
      const secret = normalizeSecret(line)
      if (!secret) at({ line: 0, name: '', reason: 'bad_secret', value: '' })
      else if (secret.length < 16) at({ line: 0, name: '', reason: 'short_secret', value: '' })
      else parsed.entries.push(fresh(secret))
    } else at({ line: 0, name: '', reason: 'unknown', value: '' })
  })
  return parsed
}

export function createStubOtp(announce: (codes: OTPCode[]) => void, nextId: () => string) {
  let entries: OTPEntry[] = []
  const codeOf = (e: OTPEntry): OTPCode => {
    const code: OTPCode = { id: e.id, name: e.name, account: e.account, kind: e.kind, digits: e.digits, period: e.period, code: '', until: 0 }
    if (e.kind === OTPKindTOTP) {
      const step = Math.floor(Date.now() / 1000 / e.period)
      code.code = fakeCode(e.secret, step, e.digits)
      code.until = (step + 1) * e.period * 1000
    }
    return code
  }
  const list = () => entries.map(codeOf)
  const changed = () => announce(list())
  const index = (id: string) => {
    const i = entries.findIndex((e) => e.id === id)
    if (i < 0) throw { code: 1, message: `there is no code of OTP "${id}"`, data: { reason: ReasonNotFound } } satisfies CoreError
    return i
  }
  return {
    list,
    /** The code {otp} stands for at a start: as the core, a HOTP code moves its counter. */
    code(id: string): OTPCode {
      const i = index(id)
      const e = entries[i]!
      const code = codeOf(e)
      if (e.kind === OTPKindHOTP) {
        code.code = fakeCode(e.secret, e.counter, e.digits)
        entries[i] = { ...e, counter: e.counter + 1 }
        changed()
      }
      return code
    },
    get: (id: string): OTPEntry => ({ ...entries[index(id)]! }),
    has: (id: string) => entries.some((e) => e.id === id),
    parse: parseOtp,
    import(text: string): OTPImported {
      const parsed = parseOtp(text)
      const result: OTPImported = { added: 0, duplicates: 0, problems: parsed.problems }
      for (const entry of parsed.entries) {
        if (entries.some((e) => e.secret === entry.secret && e.kind === entry.kind && e.name === entry.name && e.account === entry.account)) {
          result.duplicates++
          continue
        }
        entries.push({ ...entry, id: nextId() })
        result.added++
      }
      if (result.added) changed()
      return result
    },
    save(draft: OTPEntry): OTPEntry {
      const entry = { ...draft, name: draft.name.trim(), account: draft.account.trim(), secret: normalizeSecret(draft.secret) || draft.secret }
      if (entry.kind === OTPKindHOTP) entry.period = 0
      else entry.counter = 0
      const problem = problemOf(entry)
      if (problem) throw { code: 1, message: `the code of OTP cannot be saved: ${problem.reason}`, data: { reason: ReasonInvalidOTP, problem } } satisfies CoreError
      if (entry.id) entries[index(entry.id)] = entry
      else entries.push((entry.id = nextId(), entry))
      changed()
      return { ...entry }
    },
    remove(id: string): null {
      entries.splice(index(id), 1)
      changed()
      return null
    },
    move({ id, offset }: OTPMove): null {
      const i = index(id)
      const to = i + offset
      if (to >= 0 && to < entries.length) [entries[i], entries[to]] = [entries[to]!, entries[i]!]
      changed()
      return null
    },
    link(id: string): string {
      const e = entries[index(id)]!
      const label = e.account ? `${e.name}:${e.account}` : e.name
      return `otpauth://${e.kind}/${encodeURIComponent(label)}?secret=${e.secret}&issuer=${encodeURIComponent(e.name)}`
    },
  }
}
