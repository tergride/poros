import { type ComputedRef, type InjectionKey, type Ref, computed, inject, provide, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { sentence, useFailure } from '@/composables/useReason'
import { advancedActions, problemText, ruleActions } from '@/lib/rules'
import { isCoreError } from '@/platform'
import type { Preset, PresetCopy, PresetRule } from '@/platform/core-types'
import { MethodPresetsCheck, MethodPresetsGet, MethodPresetsSave } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

/**
 * The rules an editor works on, shared by the list of rules, the rule dialog and the lists window: those of a preset,
 * or of an endpoint, whose rules all go through it. The core checks every rule as it comes, so a wrong one is told at
 * once and not only at saving.
 */
export interface RuleDraft {
  /** The rules are draft.rules; an endpoint gives them in the same shape, advanced and never built-in. */
  draft: Ref<PresetCopy | null>
  endpoint: boolean
  failure: Ref<string>
  listsOpen: Ref<boolean>
  // The rule the rule dialog changes, by its place; -1 is a new one, null keeps the dialog closed.
  ruleOpen: Ref<number | null>
  /** The rules as the core keeps them, or null with the problem in failure; the draft stays as it was. */
  checked(rules: PresetRule[]): Promise<PresetRule[] | null>
  /** Puts the rules into the draft and checks them there: a wrong value stays for the user to fix. */
  apply(rules: PresetRule[]): Promise<boolean>
  /** Where a rule of the draft may send its traffic, as choices of a list; none for an endpoint. */
  targets(): { value: string; label: string }[]
}

/** The preset an editor works on: its rules, and its loading, saving and kind. */
export interface PresetDraft extends RuleDraft {
  dirty: ComputedRef<boolean>
  toAdvancedOpen: Ref<boolean>
  load(id: string): Promise<boolean>
  save(): Promise<Preset | null>
  discard(): void
  shownName(preset: Pick<Preset, 'id' | 'name' | 'builtin'> | undefined): string
}

const ruleKey: InjectionKey<RuleDraft> = Symbol('rule draft')
const presetKey: InjectionKey<PresetDraft> = Symbol('preset draft')

/** The parts of a rule draft every editor shares; a rule the core refused is named by what is wrong with it. */
export function ruleDraft(endpoint: boolean): RuleDraft & { told(error: unknown): string } {
  const core = useCoreStore()
  const failureOf = useFailure()
  const { t } = useI18n()
  const draft = ref<PresetCopy | null>(null)
  const failure = ref('')

  function told(error: unknown): string {
    if (isCoreError(error) && typeof error.data?.rule === 'number') return problemText(error.data, t)
    return failureOf(error)
  }

  async function checked(rules: PresetRule[]): Promise<PresetRule[] | null> {
    if (!draft.value) return null
    try {
      const result = await core.core().call(MethodPresetsCheck, { advanced: draft.value.advanced, endpoint, rules })
      failure.value = ''
      return result.rules
    } catch (error) {
      failure.value = told(error)
      return null
    }
  }

  return {
    draft,
    endpoint,
    failure,
    listsOpen: ref(false),
    ruleOpen: ref(null),
    checked,
    async apply(rules) {
      if (!draft.value) return false
      draft.value.rules = rules
      const result = await checked(rules)
      if (result && draft.value) draft.value.rules = result
      return result !== null
    },
    targets() {
      if (endpoint) return []
      return (draft.value?.advanced ? advancedActions : ruleActions).map((action) => ({ value: action, label: t(`action.${action}`) }))
    },
    told,
  }
}

export function provideRuleDraft(value: RuleDraft): void {
  provide(ruleKey, value)
}

export function providePresetDraft(): PresetDraft {
  const core = useCoreStore()
  const ui = useUiStore()
  const { t, te } = useI18n()
  const rules = ruleDraft(false)
  const { draft, failure, told } = rules
  const saved = ref('')

  const shownName = (preset: Pick<Preset, 'id' | 'name' | 'builtin'> | undefined) =>
    !preset ? '' : preset.builtin && te(`presets.name.${preset.id}`) ? t(`presets.name.${preset.id}`) : preset.name

  const value: PresetDraft = {
    ...rules,
    dirty: computed(() => draft.value !== null && !draft.value.builtin && JSON.stringify(draft.value) !== saved.value),
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
  // A draft of a replaced preset would save the old one back, so it goes at once, with what is open over it. At
  // once also for the route change that follows an import: there are no changes left to ask about.
  watch(
    () => ui.replacedPreset,
    (replaced) => {
      if (!replaced || draft.value?.id !== replaced.id) return
      draft.value = null
      value.listsOpen.value = false
      value.ruleOpen.value = null
      value.toAdvancedOpen.value = false
      void value.load(replaced.id)
    },
    { flush: 'sync' },
  )
  provide(presetKey, value)
  provideRuleDraft(value)
  return value
}

export function usePresetDraft(): PresetDraft {
  const draft = inject(presetKey)
  if (!draft) throw new Error('usePresetDraft needs providePresetDraft above it')
  return draft
}

export function useRuleDraft(): RuleDraft {
  const draft = inject(ruleKey)
  if (!draft) throw new Error('useRuleDraft needs a rule draft above it')
  return draft
}
