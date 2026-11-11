import { describe, expect, it } from 'vitest'

import { groupCode, hiddenCode, secondsLeft } from './otp'

describe('codes of OTP', () => {
  it('splits a code in halves, the longer last', () => {
    expect(groupCode('482913')).toBe('482 913')
    expect(groupCode('12345678')).toBe('1234 5678')
    expect(groupCode('1234567')).toBe('123 4567')
    expect(groupCode('1234')).toBe('1234')
    expect(hiddenCode(6)).toBe('••• •••')
  })

  it('counts the seconds a code has left', () => {
    expect(secondsLeft(30_000, 12_000)).toBe(18)
    expect(secondsLeft(30_000, 29_100)).toBe(1)
    expect(secondsLeft(30_000, 31_000)).toBe(0)
  })
})
