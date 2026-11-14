import { describe, expect, it } from 'vitest'

import { endingSoonMs, quotaOf } from './quota'

const now = Date.UTC(2026, 10, 9)
const gb = 1_000_000_000
const of = (used: number, total: number, expire = 0) => quotaOf({ upload: 0, download: used, total, expire }, now)

describe('the quota of a subscription', () => {
  it('runs out of traffic under a tenth of it left, and ran out at the limit', () => {
    expect(of(89 * gb, 100 * gb).traffic).toBe('ok')
    expect(of(90 * gb, 100 * gb).traffic).toBe('low')
    expect(of(100 * gb, 100 * gb).traffic).toBe('out')
    expect(of(120 * gb, 100 * gb)).toMatchObject({ traffic: 'out', percent: 100 })
  })

  it('has no traffic state without a limit and no time state without an end date', () => {
    expect(of(5 * gb, 0)).toMatchObject({ traffic: 'none', time: 'none', percent: 0 })
  })

  it('runs out of time under three days before the end, and ran out after it', () => {
    expect(of(0, 0, now + endingSoonMs + 60_000).time).toBe('ok')
    expect(of(0, 0, now + endingSoonMs).time).toBe('soon')
    expect(of(0, 0, now + 3_600_000).time).toBe('soon')
    expect(of(0, 0, now).time).toBe('over')
  })
})
