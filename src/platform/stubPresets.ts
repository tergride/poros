import type { CoreError } from './core'
import type { PresetListProblem, PresetLists, PresetListsInput, PresetListsRead, PresetRule, PresetRules, RuleProblem } from './core-types'
import {
  ActionBlock,
  ActionDirect,
  ActionEndpoint,
  ActionProxy,
  ActionWARP,
  CondDomain,
  CondIP,
  CondIPVersion,
  CondKeyword,
  CondNetwork,
  CondPort,
  CondPrivate,
  CondProcessName,
  CondProcessPath,
  CondProcessPathRegex,
  CondProtocol,
  CondRegex,
  CondSourceIP,
  CondSuffix,
  ReasonAdvancedOnly,
  ReasonDuplicate,
  ReasonEmptyCondition,
  ReasonInvalidEntry,
  ReasonInvalidValue,
  ReasonNoCondition,
  ReasonNoPrefix,
} from './core-types'

// The rules of presets as core/engine/presetrules.go and presetlists.go keep them, close enough for screens and tests.

const simpleKinds: string[] = [CondSuffix, CondIP, CondProcessName, CondProcessPath]
const ruleKinds: string[] = [
  CondSuffix, CondDomain, CondKeyword, CondRegex, CondIP, CondPrivate, CondPort, CondProcessName, CondProcessPath,
  CondProcessPathRegex, CondProtocol, CondNetwork, CondIPVersion, CondSourceIP,
]
/** The tabs of the lists window: an advanced preset has «Through WARP» too. */
export const listActions = (advanced: boolean): string[] => (advanced ? [ActionProxy, ActionDirect, ActionBlock, ActionWARP] : [ActionProxy, ActionDirect, ActionBlock])
const protocols = ['tls', 'http', 'quic', 'dns', 'stun', 'bittorrent', 'dtls', 'ssh', 'rdp', 'ntp']

function ruleFailure(problem: RuleProblem, message: string): CoreError {
  return { code: 1, message, data: problem }
}

const problemOf = (reason: string, rule: number, more: Partial<RuleProblem> = {}): RuleProblem => ({
  reason, rule, kind: '', line: 0, value: '', other: 0, ...more,
})

const isDomain = (value: string) => value.includes('.') && /^([\p{L}\p{N}_-]+\.)+[\p{L}\p{N}_-]+$/u.test(value)

/** An address or a subnet as the core writes it; null for neither. Only the plain spellings are known here. */
function prefixOf(value: string): { v4: boolean; bits: number; text: string; base: number[] } | null {
  const [addr = '', bitsText] = value.replace(/^\[|\]$/g, '').split('/')
  const v4 = /^\d{1,3}(\.\d{1,3}){3}$/.test(addr) && addr.split('.').every((p) => Number(p) <= 255)
  const v6 = !v4 && /^[0-9a-f:]+$/i.test(addr) && addr.includes(':')
  if (!v4 && !v6) return null
  const width = v4 ? 32 : 128
  const bits = bitsText === undefined ? width : Number(bitsText)
  if (!Number.isInteger(bits) || bits < 0 || bits > width) return null
  const base = v4 ? addr.split('.').map(Number) : []
  const text = bits === width ? addr.toLowerCase() : `${addr.toLowerCase()}/${bits}`
  return { v4, bits, text, base }
}

function ports(text: string): [number, number] | null {
  const [a = '', b] = text.replaceAll(':', '-').split('-')
  const from = Number(a.trim())
  const to = b === undefined ? from : Number(b.trim())
  if (!Number.isInteger(from) || !Number.isInteger(to) || from < 1 || to > 65535 || to < from) return null
  return [from, to]
}

function validRegex(value: string): boolean {
  try {
    new RegExp(value.replace(/^\(\?i\)/, ''))
    return value !== ''
  } catch {
    return false
  }
}

/** A value of a condition as the core keeps it, null for one that is not of its kind. */
export function ruleValue(kind: string, text: string): string | null {
  let value = text.trim()
  switch (kind) {
    case CondSuffix:
    case CondDomain:
      value = value.replace(/^\w+:\/\/([^/:]+).*$/, '$1').toLowerCase().replace(/\.$/, '')
      if (kind === CondSuffix) value = value.replace(/^\*\./, '').replace(/^\./, '')
      return isDomain(value) ? value : null
    case CondKeyword:
      value = value.toLowerCase()
      return value && !/\s/.test(value) ? value : null
    case CondRegex:
    case CondProcessPathRegex:
      return validRegex(value) ? value : null
    case CondIP:
    case CondSourceIP:
      return prefixOf(value)?.text ?? null
    case CondPort: {
      const range = ports(value)
      return range ? (range[0] === range[1] ? String(range[0]) : `${range[0]}-${range[1]}`) : null
    }
    case CondProcessName:
      if (!value || /[\\/*?"<>|:]/.test(value)) return null
      return /\.[a-z0-9]+$/i.test(value) ? value : `${value}.exe`
    case CondProcessPath:
      return /[\\/]/.test(value) && !/[*?"<>|]/.test(value) ? value : null
    case CondProtocol:
      return protocols.includes(value.toLowerCase()) ? value.toLowerCase() : null
    case CondNetwork:
      return ['tcp', 'udp'].includes(value.toLowerCase()) ? value.toLowerCase() : null
    case CondIPVersion:
      return value === '4' || value === '6' ? value : null
  }
  return null
}

const valueKey = (kind: string, value: string) =>
  kind === CondProcessName || kind === CondProcessPath ? `${kind} ${value.toLowerCase()}` : `${kind} ${value}`

const single = (rule: PresetRule) => rule.conditions.length === 1 && rule.conditions[0]!.values.length === 1 && !rule.invert

function contains(wide: string, narrow: string): boolean {
  const a = prefixOf(wide)
  const b = prefixOf(narrow)
  if (!a || !b || !a.v4 || !b.v4 || b.bits <= a.bits) return false
  const number = (parts: number[]) => parts.reduce((n, p) => n * 256 + p, 0)
  const block = 2 ** (32 - a.bits)
  return Math.floor(number(a.base) / block) === Math.floor(number(b.base) / block)
}

/** As narrower in core/engine/presetrules.go; addresses only for IPv4. */
function narrower(a: PresetRule, b: PresetRule): boolean {
  if (!single(a) || !single(b)) return false
  const ka = a.conditions[0]!.kind
  const va = a.conditions[0]!.values[0]!
  const kb = b.conditions[0]!.kind
  const vb = b.conditions[0]!.values[0]!
  if (kb === CondSuffix && (ka === CondSuffix || ka === CondDomain)) return va.endsWith(`.${vb}`) || (ka === CondDomain && va === vb)
  if (ka === kb && (ka === CondIP || ka === CondSourceIP)) return contains(vb, va)
  if (ka === CondPort && kb === CondPort) {
    const pa = ports(va)
    const pb = ports(vb)
    return !!pa && !!pb && pb[0] <= pa[0] && pa[1] <= pb[1] && pa[1] - pa[0] < pb[1] - pb[0]
  }
  if (ka === CondProcessPath && kb === CondProcessName) return va.split(/[\\/]/).pop()!.toLowerCase() === vb.toLowerCase()
  return false
}

export function narrowFirst(rules: PresetRule[]): PresetRule[] {
  const list = [...rules]
  for (let i = 0; i < list.length; i++) {
    const j = list.slice(0, i).findIndex((wide) => narrower(list[i]!, wide))
    if (j >= 0) list.splice(j, 0, ...list.splice(i, 1))
  }
  return list
}

/** As checkRules in core/engine/presetrules.go: the rules as kept, or a failure that names the rule and value. */
export function checkRules({ advanced, rules, endpoints = [] }: PresetRules): PresetRule[] {
  const seen = new Map<string, number>()
  const checked = rules.map((rule, i): PresetRule => {
    if ((rule.action === ActionEndpoint || rule.action === ActionWARP) && !advanced) {
      throw ruleFailure(problemOf(ReasonAdvancedOnly, i), `rule ${i + 1}: only an advanced preset sends traffic to ${rule.action}`)
    }
    if (rule.action === ActionEndpoint && !endpoints.includes(rule.endpoint)) {
      throw ruleFailure(problemOf(ReasonInvalidValue, i), `rule ${i + 1}: the preset has no endpoint "${rule.endpoint}"`)
    }
    if (rule.action !== ActionEndpoint && !listActions(true).includes(rule.action)) throw ruleFailure(problemOf(ReasonInvalidValue, i), `rule ${i + 1}: unknown action`)
    if (rule.conditions.length === 0) throw ruleFailure(problemOf(ReasonNoCondition, i), `rule ${i + 1} has no condition`)
    const name = rule.name.trim()
    if (!advanced && (rule.conditions.length > 1 || rule.conditions[0]!.values.length > 1 || rule.invert || rule.off || name)) {
      throw ruleFailure(problemOf(ReasonAdvancedOnly, i), `rule ${i + 1}: a simple preset has rules of one condition`)
    }
    const conditions = rule.conditions.map((condition, c) => {
      const kind = condition.kind
      if (!ruleKinds.includes(kind)) throw ruleFailure(problemOf(ReasonInvalidValue, i, { kind }), `there is no kind "${kind}"`)
      if (!advanced && !simpleKinds.includes(kind)) throw ruleFailure(problemOf(ReasonAdvancedOnly, i, { kind }), `${kind} is advanced`)
      if (rule.conditions.slice(0, c).some((other) => other.kind === kind)) {
        throw ruleFailure(problemOf(ReasonInvalidValue, i, { kind }), `two conditions of ${kind}`)
      }
      const values: string[] = []
      if (kind !== CondPrivate) {
        for (const [line, text] of condition.values.entries()) {
          if (!text.trim()) continue
          const value = ruleValue(kind, text)
          if (value === null) {
            throw ruleFailure(problemOf(ReasonInvalidEntry, i, { kind, line: line + 1, value: text.trim() }), `"${text}" is no ${kind}`)
          }
          if (!values.some((v) => valueKey(kind, v) === valueKey(kind, value))) values.push(value)
        }
        if (values.length === 0) throw ruleFailure(problemOf(ReasonEmptyCondition, i, { kind }), `the condition of ${kind} is empty`)
        if (kind === CondIPVersion && values.length > 1) {
          throw ruleFailure(problemOf(ReasonInvalidEntry, i, { kind, line: 2, value: values[1]! }), 'an IP version is 4 or 6')
        }
      }
      return { kind, values }
    })
    const next = { ...rule, name, conditions, endpoint: rule.action === ActionEndpoint ? rule.endpoint : '' }
    if (single(next)) {
      const key = valueKey(conditions[0]!.kind, conditions[0]!.values[0]!)
      const other = seen.get(key)
      if (other !== undefined) {
        throw ruleFailure(problemOf(ReasonDuplicate, i, { kind: conditions[0]!.kind, line: 1, value: conditions[0]!.values[0]!, other }), 'a repeated rule')
      }
      seen.set(key, i)
    }
    return next
  })
  return narrowFirst(checked)
}

const inList = (rule: PresetRule) => single(rule) && rule.conditions[0]!.kind !== CondPrivate && rule.action !== ActionEndpoint

export function writeLists({ advanced, rules }: PresetRules): PresetLists {
  const lines = new Map<string, string[]>()
  for (const rule of rules.filter(inList)) {
    if (!listActions(advanced).includes(rule.action)) throw { code: 1, message: 'unknown action', data: { reason: ReasonInvalidValue } } satisfies CoreError
    const condition = rule.conditions[0]!
    lines.set(rule.action, [...(lines.get(rule.action) ?? []), `${condition.kind}:${condition.values[0]}`])
  }
  return { lists: Object.fromEntries(listActions(advanced).map((action) => [action, (lines.get(action) ?? []).join('\n')])) }
}

export function readLists({ advanced, rules, lists }: PresetListsInput): PresetListsRead {
  for (const action of Object.keys(lists ?? {})) {
    if (!listActions(advanced).includes(action)) throw { code: 1, message: `there is no list "${action}"`, data: { reason: ReasonInvalidValue } } satisfies CoreError
  }
  const problems: PresetListProblem[] = []
  const tabOf = new Map<string, string>()
  const lines = new Map<string, { kind: string; value: string }>()
  // A line with the value of a rule to an endpoint would repeat that rule, which stays out of the window.
  for (const rule of rules) {
    if (rule.action !== ActionEndpoint || !single(rule)) continue
    const condition = rule.conditions[0]!
    tabOf.set(valueKey(condition.kind, ruleValue(condition.kind, condition.values[0]!) ?? condition.values[0]!), ActionEndpoint)
  }
  for (const action of listActions(advanced)) {
    for (const [i, raw] of (lists?.[action] ?? '').split('\n').entries()) {
      const text = raw.trim()
      if (text === '' || text.startsWith('#')) continue
      const problem = { action, line: i + 1, text, reason: '', other: '' }
      const colon = text.indexOf(':')
      const kind = colon < 0 ? undefined : ruleKinds.find((k) => k !== CondPrivate && k.toLowerCase() === text.slice(0, colon).trim().toLowerCase())
      const value = kind ? ruleValue(kind, text.slice(colon + 1)) : null
      if (!kind) problem.reason = ReasonNoPrefix
      else if (!advanced && !simpleKinds.includes(kind)) problem.reason = ReasonAdvancedOnly
      else if (value === null) problem.reason = ReasonInvalidEntry
      if (problem.reason || !kind || value === null) {
        problems.push(problem)
        continue
      }
      const key = valueKey(kind, value)
      const other = tabOf.get(key)
      if (other !== undefined) {
        if (other !== action) problems.push({ ...problem, reason: ReasonDuplicate, other })
        continue
      }
      tabOf.set(key, action)
      lines.set(key, { kind, value })
    }
  }
  const kept = new Set<string>()
  const result: PresetRule[] = []
  for (const rule of rules) {
    if (!inList(rule)) {
      result.push(rule)
      continue
    }
    const condition = rule.conditions[0]!
    const key = valueKey(condition.kind, ruleValue(condition.kind, condition.values[0]!) ?? condition.values[0]!)
    const action = tabOf.get(key)
    if (action === undefined || kept.has(key)) continue
    kept.add(key)
    result.push({ ...rule, action })
  }
  for (const [key, line] of lines) {
    if (!kept.has(key)) {
      result.push({ name: '', conditions: [{ kind: line.kind, values: [line.value] }], invert: false, off: false, action: tabOf.get(key)!, endpoint: '' })
    }
  }
  return { rules: narrowFirst(result), problems }
}
