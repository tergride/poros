import { type WritableComputedRef, computed } from 'vue'

import { useAttempt } from '@/composables/useAttempt'
import { useSettingsDraft } from '@/composables/useSettingsDraft'
import type { Settings } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

/**
 * One setting as a v-model: reading gives the core's value, writing saves it there at once. A refusal is told in
 * the notice line, and the switch shows the core's value again. Under a draft of the advanced mode the value waits
 * for «Save».
 */
export function useSetting<K extends keyof Settings>(key: K): WritableComputedRef<Settings[K]> {
  return useSettingAction(key, null)
}

/**
 * A setting whose writing does more than the core's settings, such as asking for rights first: apply runs at once,
 * or on «Save» of a draft.
 */
export function useSettingAction<K extends keyof Settings>(
  key: K,
  apply: ((value: Settings[K]) => Promise<unknown>) | null,
): WritableComputedRef<Settings[K]> {
  const core = useCoreStore()
  const attempt = useAttempt()
  const draft = useSettingsDraft()
  if (draft) {
    if (apply) draft.onSave(key, apply)
    return computed({ get: () => draft.get(key), set: (value) => draft.set(key, value) })
  }
  const write = apply ?? ((value: Settings[K]) => core.updateSettings({ [key]: value } as Partial<Settings>))
  return computed({
    get: () => core.settings![key],
    set: (value) => void attempt(() => write(value)),
  })
}
