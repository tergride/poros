import { useI18n } from 'vue-i18n'

import { isCoreError } from '@/platform'

/** The shell's own failures: no core to ask, no answer in time, a declined UAC prompt, no core started. */
export const shellReasons: Record<number, string> = {
  [-33000]: 'core_down',
  [-33001]: 'core_timeout',
  [-33002]: 'elevation_cancelled',
  [-33003]: 'core_start_failed',
}

// Reasons whose core message tells what to fix: what the provider answered, which entry is wrong.
const detailed = new Set([
  'fetch_failed',
  'invalid_subscription',
  'invalid_entry',
  'invalid_value',
  'start_failed',
  'endpoint_failed',
  'warp_failed',
  'system_proxy_failed',
  'kill_switch_failed',
  'logon_task_failed',
  'update_failed',
  'check_failed',
  'invalid_backup',
])

/** A reason as a sentence of its own: the strings are written to go inside a phrase. */
export function sentence(text: string): string {
  const trimmed = text.trim()
  if (!trimmed) return trimmed
  const capital = trimmed.charAt(0).toLocaleUpperCase() + trimmed.slice(1)
  return /[.!?…»)]$/.test(capital) ? capital : `${capital}.`
}

/** Words for a failure of the core: its reason in the interface's language, else the core's own text. */
export function useReason() {
  const { t, te } = useI18n()
  return (error: unknown): string => {
    if (isCoreError(error)) {
      const reason = error.data?.reason ?? shellReasons[error.code]
      if (reason && te(`reason.${reason}`)) return t(`reason.${reason}`)
      return error.message
    }
    return error instanceof Error ? error.message : t('reason.unknown')
  }
}

/**
 * A failure told on its own, in a banner or under a field: the reason as a sentence, then what the core said, when
 * that helps to fix it.
 */
export function useFailure() {
  const { t } = useI18n()
  const reason = useReason()
  return (error: unknown): string => {
    const text = sentence(reason(error))
    if (!isCoreError(error) || !detailed.has(error.data?.reason ?? '') || !error.message) return text
    return `${text} ${t('reason.details', { details: error.message })}`
  }
}

/** The reason of a core failure, for decisions rather than words. */
export function reasonOf(error: unknown): string {
  return isCoreError(error) ? (error.data?.reason ?? '') : ''
}
