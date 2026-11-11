import { describe, expect, it } from 'vitest'

import { qrPath } from './qr'

describe('the path of a QR code', () => {
  it('draws each run of dark modules in a row as one rectangle', () => {
    expect(qrPath(['1101', '0010'])).toBe('M0 0h2v1h-2zM3 0h1v1h-1zM2 1h1v1h-1z')
    expect(qrPath(['000'])).toBe('')
  })
})
