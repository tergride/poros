import type { Group } from '@/platform/core-types'

// A subscription runs out when under a tenth of its traffic or under three days of its time is left: its header turns
// red and says why.
export const lowTrafficShare = 0.1
export const endingSoonMs = 3 * 24 * 60 * 60 * 1000

/** none: the provider sets no limit; low and out: running out and ran out. */
export type TrafficState = 'none' | 'ok' | 'low' | 'out'
/** none: no end date; soon and over: ending and ended. */
export type TimeState = 'none' | 'ok' | 'soon' | 'over'

export interface Quota {
  used: number
  total: number
  /** How full the bar is, from 0 to 100. */
  percent: number
  traffic: TrafficState
  time: TimeState
}

export function quotaOf(group: Pick<Group, 'upload' | 'download' | 'total' | 'expire'>, now: number): Quota {
  const used = group.upload + group.download
  const { total, expire } = group
  let traffic: TrafficState = 'none'
  if (total > 0) traffic = used >= total ? 'out' : total - used <= total * lowTrafficShare ? 'low' : 'ok'
  let time: TimeState = 'none'
  if (expire > 0) time = expire <= now ? 'over' : expire - now <= endingSoonMs ? 'soon' : 'ok'
  return { used, total, percent: total > 0 ? Math.min(100, (used / total) * 100) : 0, traffic, time }
}
