import { describe, expect, it } from 'vitest'

import { filterLog, logLevel } from './logs'

const lines = [
  '2026/11/10 14:02:11 connection: running',
  '+0300 2026-11-10 14:05:47 WARN outbound/vless[proxy]: stream reset',
  'ERROR[0000] dns: exchange tcp://8.8.8.8:53: i/o timeout',
  '2026/11/10 14:20:00 [Warning] xray: the leg is slow',
  '+0300 2026-11-10 14:21:00 DEBUG router: match rule 3',
]

describe('the log screen', () => {
  it('weighs the lines of Poros, sing-box and Xray', () => {
    expect(lines.map(logLevel)).toEqual(['info', 'warn', 'error', 'warn', 'debug'])
  })

  it('lets a level through with the heavier ones', () => {
    const all = { text: '', matchCase: false, regex: false }
    expect(filterLog(lines, { ...all, level: 'warn' }).lines).toEqual([lines[1], lines[2], lines[3]])
    expect(filterLog(lines, { ...all, level: 'error' }).lines).toEqual([lines[2]])
    expect(filterLog(lines, { ...all, level: 'all' }).lines).toHaveLength(5)
  })

  it('filters by text, in its case or any, and by a regular expression', () => {
    expect(filterLog(lines, { text: 'DNS', matchCase: false, regex: false, level: 'all' }).lines).toEqual([lines[2]])
    expect(filterLog(lines, { text: 'DNS', matchCase: true, regex: false, level: 'all' }).lines).toEqual([])
    expect(filterLog(lines, { text: '\\d+\\.\\d+\\.\\d+\\.\\d+:53', matchCase: false, regex: true, level: 'all' }).lines).toEqual([lines[2]])
  })

  it('keeps the lines while a regular expression does not compile', () => {
    expect(filterLog(lines, { text: '(', matchCase: false, regex: true, level: 'all' })).toEqual({ lines, invalid: true })
  })
})
