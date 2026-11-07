import { type WritableComputedRef, computed } from 'vue'

import { useAttempt } from '@/composables/useAttempt'
import type { Settings } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

/**
 * One setting as a v-model: reading gives the core's value, writing saves it there at once. A refusal is told in
 * the notice line, and the switch shows the core's value again.
 */
export function useSetting<K extends keyof Settings>(key: K): WritableComputedRef<Settings[K]> {
  const core = useCoreStore()
  const attempt = useAttempt()
  return computed({
    get: () => core.settings![key],
    set: (value) => void attempt(() => core.updateSettings({ [key]: value } as Partial<Settings>)),
  })
}
