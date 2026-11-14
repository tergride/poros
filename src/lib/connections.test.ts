import { describe, expect, it } from 'vitest'

import type { Connection } from '@/platform/core-types'

import { filterConnections, groupByProcess, withSpeeds } from './connections'

const base: Connection = {
  id: '',
  start: 0,
  network: 'tcp',
  protocol: 'tls',
  process: '',
  processPath: '',
  source: '127.0.0.1:50000',
  destination: '',
  inbound: 'mixed-in',
  outbound: 'proxy',
  rule: '',
  upload: 0,
  download: 0,
}
const connections: Connection[] = [
  { ...base, id: 'a', start: 1, process: 'chrome.exe', destination: 'www.example.com:443', upload: 100, download: 900 },
  { ...base, id: 'b', start: 2, process: 'chrome.exe', destination: 'cdn.example.net:443', network: 'udp', protocol: 'quic', download: 50 },
  { ...base, id: 'c', start: 3, process: 'steam.exe', destination: 'cache1.example.com:443', outbound: 'direct', download: 4000 },
  { ...base, id: 'd', start: 4, destination: '1.1.1.1:53', network: 'udp', protocol: 'dns', outbound: 'direct', download: 10 },
]

describe('the connections screen', () => {
  it('takes the speed from what moved since the last poll', () => {
    const rows = withSpeeds(connections, new Map([['a', 500], ['c', 4000]]), 2)
    expect(rows.map((r) => r.speed)).toEqual([250, 25, 0, 5])
  })

  it('groups by program, the busiest first, the system without a name', () => {
    const groups = groupByProcess(withSpeeds(connections, new Map(), 1))
    expect(groups.map((g) => [g.process, g.connections.length, g.traffic])).toEqual([
      ['steam.exe', 1, 4000],
      ['chrome.exe', 2, 1050],
      ['', 1, 10],
    ])
    expect(groups[1]!.connections.map((c) => c.id)).toEqual(['b', 'a'])
    expect(groups[1]!.networks).toEqual(['tcp', 'udp'])
  })

  it('filters by program, address or the name of the outbound', () => {
    const rows = withSpeeds(connections, new Map(), 1)
    const name = (tag: string) => (tag === 'proxy' ? 'Germany' : 'Напрямую')
    expect(filterConnections(rows, 'CHROME', name).map((r) => r.id)).toEqual(['a', 'b'])
    expect(filterConnections(rows, 'germany', name).map((r) => r.id)).toEqual(['a', 'b'])
    expect(filterConnections(rows, ':53', name).map((r) => r.id)).toEqual(['d'])
    expect(filterConnections(rows, ' ', name)).toHaveLength(4)
  })
})
