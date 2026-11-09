import { describe, expect, it } from 'vitest'

import { inRow, kindOfLine, moved, newRule, problemText, ruleSummary } from './rules'

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
})
