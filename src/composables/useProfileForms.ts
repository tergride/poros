import { shallowRef } from 'vue'

import type { ServerForms } from '@/platform/core-types'
import { MethodServersForms } from '@/platform/core-types'
import type { ProfileForm } from '@/platform/profile-types'
import { useCoreStore } from '@/stores/core'

const forms = shallowRef<ServerForms | null>(null)
let loading: Promise<ServerForms> | null = null

/** What the profile editor draws: the core describes it once, and it does not change while the core runs. */
export function useProfileForms() {
  const core = useCoreStore()
  function load(): Promise<ServerForms> {
    loading ??= core
      .core()
      .call(MethodServersForms)
      .then((loaded) => (forms.value = loaded))
      .catch((error: unknown) => {
        loading = null
        throw error
      })
    return loading
  }
  const formOf = (type: string): ProfileForm | undefined => forms.value?.forms.find((f) => f.type === type)
  return { forms, load, formOf }
}
