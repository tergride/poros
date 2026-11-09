import { type ComputedRef, type InjectionKey, type Ref, computed, inject, provide, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { sentence, useFailure } from '@/composables/useReason'
import { problemText } from '@/lib/rules'
import { isCoreError } from '@/platform'
import type { Preset, PresetCopy, PresetRule } from '@/platform/core-types'
import { MethodPresetsCheck, MethodPresetsGet, MethodPresetsSave } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

/**
 * The preset an editor works on, shared by the parts of the editor: the draft, what is wrong with it, and the dialogs
 * around it. The core checks every rule as it comes, so a wrong one is told at once and not only at saving.
 */
export interface PresetDraft {
  draft: Ref<PresetCopy | null>
  failure: Ref<string>
  dirty: ComputedRef<boolean>
  listsOpen: Ref<boolean>
  // The rule the rule dialog changes, by its place; -1 is a new one, null keeps the dialog closed.
  ruleOpen: Ref<number | null>
  toAdvancedOpen: Ref<boolean>
  load(id: string): Promise<boolean>
  /** The rules as the core keeps them, or null with the problem in failure; the draft stays as it was. */
  checked(rules: PresetRule[]): Promise<PresetRule[] | null>
  /** Puts the rules into the draft and checks them there: a wrong value stays for the user to fix. */
  apply(rules: PresetRule[]): Promise<boolean>
  save(): Promise<Preset | null>
  discard(): void
  shownName(preset: Pick<Preset, 'id' | 'name' | 'builtin'> | undefined): string
}

const draftKey: InjectionKey<PresetDraft> = Symbol('preset draft')

export function providePresetDraft(): PresetDraft {
  const core = useCoreStore()
  const failureOf = useFailure()
  const { t, te } = useI18n()
  const draft = ref<PresetCopy | null>(null)
  const saved = ref('')
  const failure = ref('')

  const shownName = (preset: Pick<Preset, 'id' | 'name' | 'builtin'> | undefined) =>
    !preset ? '' : preset.builtin && te(`presets.name.${preset.id}`) ? t(`presets.name.${preset.id}`) : preset.name

  // A rule the core refused is named by what is wrong with it; anything else by its reason.
  function told(error: unknown): string {
    if (isCoreError(error) && typeof error.data?.rule === 'number') return problemText(error.data, t)
    return failureOf(error)
  }

  async function checked(rules: PresetRule[]): Promise<PresetRule[] | null> {
    if (!draft.value) return null
    try {
      const result = await core.core().call(MethodPresetsCheck, { advanced: draft.value.advanced, rules })
      failure.value = ''
      return result.rules
    } catch (error) {
      failure.value = told(error)
      return null
    }
  }

  const value: PresetDraft = {
    draft,
    failure,
    dirty: computed(() => draft.value !== null && !draft.value.builtin && JSON.stringify(draft.value) !== saved.value),
    listsOpen: ref(false),
    ruleOpen: ref(null),
    toAdvancedOpen: ref(false),
    async load(id) {
      failure.value = ''
      try {
        const copy = await core.core().call(MethodPresetsGet, { id })
        draft.value = copy
        saved.value = JSON.stringify(copy)
        return true
      } catch (error) {
        draft.value = null
        failure.value = told(error)
        return false
      }
    },
    checked,
    async apply(rules) {
      if (!draft.value) return false
      draft.value.rules = rules
      const result = await checked(rules)
      if (result && draft.value) draft.value.rules = result
      return result !== null
    },
    async save() {
      const copy = draft.value
      if (!copy || copy.builtin) return null
      failure.value = ''
      // The core knows the built-in presets by their English names; the ones the user sees are checked here.
      const name = copy.name.trim().toLowerCase()
      if (core.presets.some((p) => p.id !== copy.id && shownName(p).trim().toLowerCase() === name)) {
        failure.value = sentence(t('reason.preset_name_taken'))
        return null
      }
      try {
        const preset = await core.core().call(MethodPresetsSave, copy)
        await value.load(preset.id)
        return preset
      } catch (error) {
        failure.value = told(error)
        return null
      }
    },
    discard() {
      if (saved.value) draft.value = JSON.parse(saved.value) as PresetCopy
      failure.value = ''
    },
    shownName,
  }
  provide(draftKey, value)
  return value
}

export function usePresetDraft(): PresetDraft {
  const draft = inject(draftKey)
  if (!draft) throw new Error('usePresetDraft needs providePresetDraft above it')
  return draft
}
