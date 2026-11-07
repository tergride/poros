import { useFailure } from '@/composables/useReason'
import { useUiStore } from '@/stores/ui'

/**
 * Runs what the user asked for and tells its failure in the notice line instead of dropping it: a button, a switch
 * or a tray item whose call fails must not look as if it did nothing.
 */
export function useAttempt() {
  const ui = useUiStore()
  const failure = useFailure()
  return async (action: () => Promise<unknown>): Promise<void> => {
    try {
      await action()
    } catch (error) {
      ui.notice = { tone: 'warn', text: failure(error) }
    }
  }
}
