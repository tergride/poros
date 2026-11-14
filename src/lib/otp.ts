/** A code in two halves, as authenticator apps show it: 482 913, 1234 5678; the longer half comes last. */
export function groupCode(code: string): string {
  if (code.length < 6) return code
  const half = Math.floor(code.length / 2)
  return `${code.slice(0, half)} ${code.slice(half)}`
}

/** Whole seconds until a code gives way to the next one, at least 1 while it lasts. */
export function secondsLeft(until: number, now: number): number {
  return Math.max(0, Math.ceil((until - now) / 1000))
}

/** A placeholder of a code not shown, as wide as the code: ••• •••. */
export function hiddenCode(digits: number): string {
  return groupCode('•'.repeat(digits))
}
