import { useI18n } from 'vue-i18n'

import { isCoreError } from '@/platform'

/** Words for a failure of the core: its reason in the interface's language, else the core's own text. */
export function useReason() {
  const { t, te } = useI18n()
  return (error: unknown): string => {
    if (isCoreError(error)) {
      const reason = error.data?.reason
      if (reason && te(`reason.${reason}`)) return t(`reason.${reason}`)
      return error.message
    }
    return error instanceof Error ? error.message : t('reason.unknown')
  }
}

/** The reason of a core failure, for decisions rather than words. */
export function reasonOf(error: unknown): string {
  return isCoreError(error) ? (error.data?.reason ?? '') : ''
}
