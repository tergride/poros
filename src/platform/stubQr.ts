import type { CoreError } from './core'
import type { QRCode, QRScan } from './core-types'
import { ReasonQRTooLong } from './core-types'

// QR codes of the stub core: a matrix of the size a real one would have, with its finders, and modules from a hash of
// the text, enough for the interface to draw. The screen holds what Playwright puts into window.porosScreenQr.

const capacity = 2953

export function stubQrEncode(text: string): QRCode {
  const bytes = new TextEncoder().encode(text).length
  if (bytes > capacity) throw { code: 1, message: 'the text is longer than a QR code holds', data: { reason: ReasonQRTooLong } } satisfies CoreError
  const version = Math.min(40, 1 + Math.floor(bytes / 70))
  const n = 17 + 4 * version
  let x = 2166136261
  for (const c of text) x = Math.imul(x ^ c.charCodeAt(0), 16777619) >>> 0
  const finder = (r: number, c: number): boolean | null => {
    for (const [top, left] of [[0, 0], [0, n - 7], [n - 7, 0]] as const) {
      const a = r - top
      const b = c - left
      if (a >= -1 && a <= 7 && b >= -1 && b <= 7) {
        if (a < 0 || a > 6 || b < 0 || b > 6) return false
        return a % 6 === 0 || b % 6 === 0 || (a >= 2 && a <= 4 && b >= 2 && b <= 4)
      }
    }
    return null
  }
  const rows: string[] = []
  for (let r = 0; r < n; r++) {
    let row = ''
    for (let c = 0; c < n; c++) {
      x = Math.imul(x ^ (r * 131 + c), 16777619) >>> 0
      row += (finder(r, c) ?? (x & 4) !== 0) ? '1' : '0'
    }
    rows.push(row)
  }
  return { rows }
}

export function stubQrScan(): QRScan {
  const texts = (globalThis as { porosScreenQr?: string[] }).porosScreenQr ?? []
  return { texts: [...texts] }
}
