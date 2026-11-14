import type { Connection } from '@/platform/core-types'

/** A connection with its speed: bytes a second over the last poll. */
export interface ConnectionRow extends Connection {
  speed: number
}

/** A program and its connections, with their traffic and speed summed. Process "" is the system's. */
export interface ProcessGroup {
  process: string
  connections: ConnectionRow[]
  traffic: number
  speed: number
  networks: string[]
  outbounds: string[]
}

/**
 * The speeds of connections from two polls `seconds` apart: what each moved since the last one. A connection new
 * since then moved all its bytes in that time.
 */
export function withSpeeds(connections: Connection[], before: Map<string, number>, seconds: number): ConnectionRow[] {
  return connections.map((c) => {
    const total = c.upload + c.download
    const previous = before.get(c.id) ?? 0
    return { ...c, speed: seconds > 0 ? Math.max(0, total - previous) / seconds : 0 }
  })
}

/** The connections a filter lets through: by program, address or outbound, in any case. */
export function filterConnections(rows: ConnectionRow[], text: string, outboundName: (tag: string) => string): ConnectionRow[] {
  const needle = text.trim().toLowerCase()
  if (!needle) return rows
  return rows.filter((c) =>
    [c.process, c.destination, c.source, c.outbound, outboundName(c.outbound)].some((field) => field.toLowerCase().includes(needle)),
  )
}

/** The connections by program, the busiest first; within a program the newest first. */
export function groupByProcess(rows: ConnectionRow[]): ProcessGroup[] {
  const groups = new Map<string, ProcessGroup>()
  for (const c of rows) {
    const key = c.process.toLowerCase()
    let group = groups.get(key)
    if (!group) {
      group = { process: c.process, connections: [], traffic: 0, speed: 0, networks: [], outbounds: [] }
      groups.set(key, group)
    }
    group.connections.push(c)
    group.traffic += c.upload + c.download
    group.speed += c.speed
    if (!group.networks.includes(c.network)) group.networks.push(c.network)
    if (!group.outbounds.includes(c.outbound)) group.outbounds.push(c.outbound)
  }
  const list = [...groups.values()]
  for (const group of list) group.connections.sort((a, b) => b.start - a.start)
  return list.sort((a, b) => b.speed - a.speed || b.traffic - a.traffic || a.process.localeCompare(b.process))
}
