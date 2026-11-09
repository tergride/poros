import type { Field, FieldCondition, Profile, ProfileForm } from '@/platform/profile-types'

type Json = Record<string, unknown>

const isObject = (value: unknown): value is Json => typeof value === 'object' && value !== null && !Array.isArray(value)

/** The value at a path of JSON keys joined by dots, as the core's field descriptions name it. */
export function getAt(root: unknown, path: string): unknown {
  let value = root
  for (const key of path.split('.')) {
    if (!isObject(value)) return undefined
    value = value[key]
  }
  return value
}

/** Whether a value is set: the core stores the profile without empty values, so these are the same as missing. */
export function isSet(value: unknown): boolean {
  if (value === undefined || value === null || value === false || value === '' || value === 0) return false
  if (Array.isArray(value)) return value.length > 0
  if (isObject(value)) return Object.keys(value).length > 0
  return true
}

/**
 * Puts a value at a path, creating the objects on the way. An empty value removes the key, and objects it leaves
 * empty go too, down to the protocol's own part, which a profile always carries.
 */
export function setAt(root: Json, path: string, value: unknown): void {
  const keys = path.split('.')
  const nodes: Json[] = [root]
  let node = root
  for (const key of keys.slice(0, -1)) {
    let next = node[key]
    if (!isObject(next)) {
      if (!isSet(value)) return
      next = {}
      node[key] = next
    }
    node = next as Json
    nodes.push(node)
  }
  const last = keys[keys.length - 1]!
  if (isSet(value)) node[last] = value
  else delete node[last]
  for (let i = keys.length - 1; i >= 2 && Object.keys(nodes[i]!).length === 0; i--) delete nodes[i - 1]![keys[i - 1]!]
}

const asText = (value: unknown): string => (value === undefined || value === null || value === false ? '' : String(value))

export function holds(condition: FieldCondition, profile: unknown): boolean {
  const value = getAt(profile, condition.path)
  const result = condition.in?.length ? condition.in.includes(asText(value)) : isSet(value)
  return condition.not ? !result : result
}

/**
 * How a field stands with the rest of the profile: active, inactive while a setting it depends on is off, or hidden
 * when a choice of another field rules it out, as a transport rules out the fields of the others.
 */
export type FieldState = 'active' | 'inactive' | 'hidden'

export function fieldState(field: Field, profile: unknown): FieldState {
  let state: FieldState = 'active'
  for (const condition of field.when ?? []) {
    if (holds(condition, profile)) continue
    if (condition.in?.length) return 'hidden'
    state = 'inactive'
  }
  return state
}

/** The TLS object at a path as one choice: none, TLS or REALITY. */
export function securityOf(profile: unknown, path: string): string {
  if (getAt(profile, `${path}.reality.enabled`)) return 'reality'
  return getAt(profile, `${path}.enabled`) ? 'tls' : ''
}

export function setSecurity(profile: Json, path: string, choice: string): void {
  setAt(profile, `${path}.enabled`, choice !== '')
  setAt(profile, `${path}.reality.enabled`, choice === 'reality')
}

/**
 * The profile without the values of inactive and hidden fields: the core would read them, and a setting that does
 * not apply must not change the server, as ECH would under REALITY. Fields of other types are kept as they are.
 */
export function cleanProfile(form: ProfileForm | undefined, profile: Profile): Profile {
  // A copy through JSON: the profile may be reactive, which structuredClone refuses.
  const clean = JSON.parse(JSON.stringify(profile)) as Json
  if (!form) return clean as unknown as Profile
  for (let pass = 0; pass < 4; pass++) {
    const states = form.fields.map((f) => fieldState(f, clean))
    const active = new Set(form.fields.filter((_, i) => states[i] === 'active').map((f) => f.path))
    const unused = form.fields.filter((f, i) => states[i] !== 'active' && !active.has(f.path) && isSet(getAt(clean, f.path)))
    if (unused.length === 0) break
    for (const f of unused) setAt(clean, f.path, undefined)
  }
  return clean as unknown as Profile
}

/** HTTP headers as one "Name: value" per line. */
export function headersText(value: unknown): string {
  return isObject(value)
    ? Object.entries(value)
        .map(([name, v]) => `${name}: ${String(v)}`)
        .join('\n')
    : ''
}

export function parseHeaders(text: string): Record<string, string> {
  const headers: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const cut = line.indexOf(':')
    const name = (cut < 0 ? line : line.slice(0, cut)).trim()
    if (name) headers[name] = cut < 0 ? '' : line.slice(cut + 1).trim()
  }
  return headers
}

export function parseList(text: string): string[] {
  return text
    .split(/[,\n]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

/** Numbers from 0 to 255 written apart by commas or spaces; undefined when one of them is not such a number. */
export function parseBytes(text: string): number[] | undefined {
  const items = text.split(/[\s,]+/).filter(Boolean)
  const bytes = items.map(Number)
  return bytes.every((b) => Number.isInteger(b) && b >= 0 && b <= 255) ? bytes : undefined
}

/** Where JSON.parse stopped, as a line of the text, for the error to point at. */
export function jsonErrorLine(text: string, error: unknown): number | null {
  const message = error instanceof Error ? error.message : ''
  const line = /line (\d+)/.exec(message)
  if (line) return Number(line[1])
  const position = /position (\d+)/.exec(message)
  if (position) return text.slice(0, Number(position[1])).split('\n').length
  return null
}
