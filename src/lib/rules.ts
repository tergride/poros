import type { PresetRule, RuleProblem } from '@/platform/core-types'
import {
  ActionBlock,
  ActionDirect,
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
  ReasonNoCondition,
} from '@/platform/core-types'

/** The kinds of a rule of a simple preset, and of an advanced one, in the order the lists show them. */
export const simpleKinds: string[] = [CondSuffix, CondIP, CondProcessName, CondProcessPath]
export const advancedKinds: string[] = [
  ...simpleKinds,
  CondDomain,
  CondKeyword,
  CondRegex,
  CondPort,
  CondProcessPathRegex,
  CondProtocol,
  CondNetwork,
  CondIPVersion,
  CondSourceIP,
  CondPrivate,
]

/** Where a rule of a preset sends its traffic; an advanced preset adds WARP. A rule of an endpoint goes through it. */
export const ruleActions: string[] = [ActionProxy, ActionDirect, ActionBlock]
export const advancedActions: string[] = [...ruleActions, ActionWARP]

/** The protocols sing-box recognizes, as the core checks them. */
export const protocols = ['tls', 'http', 'quic', 'dns', 'stun', 'bittorrent', 'dtls', 'ssh', 'rdp', 'ntp']

/** A rule of one condition. */
export function newRule(kind: string, value: string, action: string): PresetRule {
  return { name: '', conditions: [{ kind, values: kind === CondPrivate || value === '' ? [] : [value] }], invert: false, off: false, action }
}

/** A rule a row of the list edits in place: one condition with at most one value, not inverted. */
export function inRow(rule: PresetRule): boolean {
  const condition = rule.conditions[0]
  return rule.conditions.length === 1 && !!condition && condition.values.length <= 1 && !rule.invert
}

/** Moves the rule at `from` by one place up (-1) or down (1). */
export function moved(rules: PresetRule[], from: number, by: -1 | 1): PresetRule[] {
  const to = from + by
  if (to < 0 || to >= rules.length) return rules
  const next = [...rules]
  ;[next[from], next[to]] = [next[to]!, next[from]!]
  return next
}

type Translate = (key: string, values?: Record<string, unknown>) => string

/** The value of a condition as a person reads it: the protocols and networks in capitals, as their names are. */
function shownValue(kind: string, value: string): string {
  if (kind === CondNetwork) return value.toUpperCase()
  if (kind === CondProtocol) return value === 'bittorrent' ? 'BitTorrent' : value.toUpperCase()
  return value
}

/** What a rule of several conditions says, for its row: its name, then each condition with its values. */
export function ruleSummary(rule: PresetRule, t: Translate): string {
  const parts = rule.conditions.map((c) => {
    const kind = t(`rules.kinds.${c.kind}`).toLocaleLowerCase()
    return c.values.length > 0 ? `${kind} ${c.values.map((v) => shownValue(c.kind, v)).join(', ')}` : kind
  })
  const text = parts.join(' · ')
  const what = rule.invert ? t('rules.invertedSummary', { text }) : text
  return rule.name ? `${rule.name}: ${what}` : what
}

/** Words for a problem the core found in a rule. */
export function problemText(problem: Partial<RuleProblem>, t: Translate): string {
  const kind = problem.kind ?? ''
  switch (problem.reason ?? '') {
    case ReasonInvalidEntry: {
      const why = t(`rules.invalid.${kind}`)
      // A value of a list in the rule window says which line it is on.
      return problem.line && problem.line > 1
        ? t('rules.problem.line', { kind: t(`rules.kinds.${kind}`), line: problem.line, value: problem.value ?? '', why })
        : t('rules.problem.value', { value: problem.value ?? '', why })
    }
    case ReasonDuplicate:
      return t('rules.problem.duplicate', { value: problem.value ?? '' })
    case ReasonNoCondition:
      return t('rules.problem.noCondition')
    case ReasonEmptyCondition:
      return t('rules.problem.empty', { kind: t(`rules.kinds.${kind}`) })
    case ReasonAdvancedOnly:
      return t('rules.problem.advanced')
  }
  return t('rules.problem.invalid')
}

/** The kind a line of the lists window names by its prefix, whatever its case; "" for none. */
export function kindOfLine(text: string): string {
  const colon = text.indexOf(':')
  if (colon < 0) return ''
  const prefix = text.slice(0, colon).trim().toLowerCase()
  return advancedKinds.find((kind) => kind !== CondPrivate && kind.toLowerCase() === prefix) ?? ''
}

/** The kinds a line of the lists window can name: the private networks have no line. */
export function listKinds(advanced: boolean): string[] {
  return (advanced ? advancedKinds : simpleKinds).filter((kind) => kind !== CondPrivate)
}

/**
 * The prefix being typed at the caret of the lists window: where it starts after the indent of the line, and what is
 * typed of it up to the caret. A line with a colon has its prefix already: null.
 */
export function prefixAt(text: string, caret: number): { from: number; typed: string } | null {
  const start = caret > 0 ? text.lastIndexOf('\n', caret - 1) + 1 : 0
  const end = text.indexOf('\n', caret)
  if (text.slice(start, end < 0 ? undefined : end).includes(':')) return null
  const typed = text.slice(start, caret).trimStart()
  return { from: caret - typed.length, typed }
}

/** The kinds whose prefix fits what is typed, whatever its case: those it begins first, then those it is part of. */
export function prefixesFor(kinds: string[], typed: string): string[] {
  const query = typed.toLowerCase()
  const begin = kinds.filter((kind) => kind.toLowerCase().startsWith(query))
  return [...begin, ...kinds.filter((kind) => !begin.includes(kind) && kind.toLowerCase().includes(query))]
}
