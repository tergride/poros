import { describe, expect, it } from 'vitest'

import { speedPoints, speedSeconds, speedTicks } from './speedChart'

const KB = 1024
const MB = 1024 ** 2

describe('the speed chart', () => {
  it('keeps a quiet minute on a scale of tens of kilobytes', () => {
    expect(speedTicks(0)).toEqual([0, 10, 20, 30, 40].map((n) => n * KB))
    expect(speedTicks(300)).toEqual(speedTicks(0))
  })

  it('rounds the top up to a step of 1, 2, 2,5 or 5, at most four steps', () => {
    expect(speedTicks(1.82 * MB)).toEqual([0, 0.5, 1, 1.5, 2].map((n) => n * MB))
    expect(speedTicks(4 * MB)).toEqual([0, 1, 2, 3, 4].map((n) => n * MB))
    expect(speedTicks(4.1 * MB)).toEqual([0, 2, 4, 6].map((n) => n * MB))
    expect(speedTicks(700 * KB)).toEqual([0, 200, 400, 600, 800].map((n) => n * KB))
  })

  it('writes a top of a thousand units and more in the next unit', () => {
    expect(speedTicks(900 * KB)).toEqual([0, 256, 512, 768, 1024].map((n) => n * KB))
    expect(speedTicks(1023 * MB)).toEqual([0, 0.25, 0.5, 0.75, 1].map((n) => n * 1024 * MB))
  })

  it('puts the newest sample at the right edge, a slot a second', () => {
    expect(speedPoints([0, 50, 100], 100, 600, 100)).toEqual([
      [580, 100],
      [590, 50],
      [600, 0],
    ])
    const full = speedPoints(Array.from({ length: speedSeconds + 5 }, () => 10), 100, 600, 100)
    expect(full).toHaveLength(speedSeconds + 1)
    expect(full[0]).toEqual([0, 90])
  })
})
