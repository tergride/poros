import { describe, expect, it } from 'vitest'

import { fromTarget, inRow, kindOfLine, listKinds, moved, newRule, prefixAt, prefixesFor, problemText, ruleSummary, targetOf } from './rules'

// The keys and values as the messages of the interface would put them together.
const t = (key: string, values: Record<string, unknown> = {}) =>
  `${key}${Object.keys(values).length ? JSON.stringify(values) : ''}`

describe('the rules of a preset in the interface', () => {
  it('edits in a row only a rule of one condition and one value', () => {
    expect(inRow(newRule('suffix', 'example.com', 'proxy'))).toBe(true)
    expect(inRow(newRule('private', '', 'direct'))).toBe(true)
    expect(inRow({ ...newRule('suffix', 'example.com', 'proxy'), invert: true })).toBe(false)
    const two = newRule('processName', 'game.exe', 'direct')
    two.conditions.push({ kind: 'network', values: ['udp'] })
    expect(inRow(two)).toBe(false)
  })

  it('makes one choice of an action or an endpoint, and back', () => {
    const office = newRule('ip', '10.8.0.0/16', 'endpoint:a1b2')
    expect(office).toMatchObject({ action: 'endpoint', endpoint: 'a1b2' })
    expect(targetOf(office)).toBe('endpoint:a1b2')
    expect(fromTarget('direct')).toEqual({ action: 'direct', endpoint: '' })
  })

  it('moves a rule by one place and stays at the ends', () => {
    const rules = ['a', 'b', 'c'].map((v) => newRule('suffix', `${v}.example`, 'proxy'))
    expect(moved(rules, 2, -1).map((r) => r.conditions[0]!.values[0])).toEqual(['a.example', 'c.example', 'b.example'])
    expect(moved(rules, 0, -1)).toBe(rules)
    expect(moved(rules, 2, 1)).toBe(rules)
  })

  it('sums up a rule of several conditions with its name', () => {
    const rule = newRule('processName', 'game.exe', 'direct')
    rule.conditions[0]!.values.push('launcher.exe')
    rule.conditions.push({ kind: 'network', values: ['udp'] }, { kind: 'private', values: [] })
    rule.name = 'Games'
    expect(ruleSummary(rule, (key) => key.replace('rules.kinds.', 'K-'))).toBe('Games: k-processname game.exe, launcher.exe · k-network UDP · k-private')
  })

  it('names the line of a list value and the value alone otherwise', () => {
    expect(problemText({ reason: 'invalid_entry', kind: 'ip', line: 2, value: '10.8.0.0/33' }, t)).toBe(
      'rules.problem.line{"kind":"rules.kinds.ip","line":2,"value":"10.8.0.0/33","why":"rules.invalid.ip"}',
    )
    expect(problemText({ reason: 'invalid_entry', kind: 'suffix', line: 1, value: 'not a site' }, t)).toBe(
      'rules.problem.value{"value":"not a site","why":"rules.invalid.suffix"}',
    )
    expect(problemText({ reason: 'duplicate', value: 'example.com' }, t)).toBe('rules.problem.duplicate{"value":"example.com"}')
    expect(problemText({ reason: 'invalid_value' }, t)).toBe('rules.problem.invalid')
  })

  it('reads the kind of a line of the lists window by its prefix', () => {
    expect(kindOfLine('SUFFIX:example.com')).toBe('suffix')
    expect(kindOfLine('processPathRegex: (?i)games')).toBe('processPathRegex')
    expect(kindOfLine('private:')).toBe('')
    expect(kindOfLine('example.com')).toBe('')
  })

  it('offers prefixes on a line until it has a colon, from its indent to the caret', () => {
    expect(prefixAt('', 0)).toEqual({ from: 0, typed: '' })
    expect(prefixAt('\nsuffix:example.com', 0)).toEqual({ from: 0, typed: '' })
    expect(prefixAt('suffix:example.com\n  pro', 24)).toEqual({ from: 21, typed: 'pro' })
    // A value without its prefix takes one before it.
    expect(prefixAt('example.com', 0)).toEqual({ from: 0, typed: '' })
    expect(prefixAt('suffix:example.com', 3)).toBeNull()
    expect(prefixAt('suffix:example.com\nip', 7)).toBeNull()
  })

  it('narrows the prefixes to the typed text, those it begins first', () => {
    expect(listKinds(false)).toEqual(['suffix', 'ip', 'processName', 'processPath'])
    expect(listKinds(true)).not.toContain('private')
    expect(prefixesFor(listKinds(false), '')).toEqual(listKinds(false))
    expect(prefixesFor(listKinds(false), 'PRO')).toEqual(['processName', 'processPath'])
    expect(prefixesFor(listKinds(true), 'ip')).toEqual(['ip', 'ipVersion', 'sourceIP'])
    expect(prefixesFor(listKinds(true), 'regex')).toEqual(['regex', 'processPathRegex'])
    expect(prefixesFor(listKinds(true), 'example.com')).toEqual([])
  })
})
