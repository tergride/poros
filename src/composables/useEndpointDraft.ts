import { type ComputedRef, type InjectionKey, type Ref, computed, inject, provide, ref, watch } from 'vue'

import { type RuleDraft, provideRuleDraft, ruleDraft } from '@/composables/usePresetDraft'
import { useProfileForms } from '@/composables/useProfileForms'
import { cleanProfile } from '@/lib/profileForm'
import type { Endpoint, EndpointEntry } from '@/platform/core-types'
import { ActionProxy, MethodEndpointsGet, MethodEndpointsSave } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

/**
 * The endpoint its editor works on, shared by the parts of the editor: its connection and switch in entry, its rules in
 * the rule draft the list of rules and the rule dialog take, and what is wrong with it. A new endpoint is a draft with
 * no ID until it is saved.
 */
export interface EndpointDraft {
  entry: Ref<EndpointEntry | null>
  rules: RuleDraft
  failure: Ref<string>
  dirty: ComputedRef<boolean>
  listsOpen: Ref<boolean>
  ruleOpen: Ref<number | null>
  load(id: string): Promise<boolean>
  start(entry: EndpointEntry): void
  /** A new draft with the connection and the rules of this one under name, off. */
  duplicate(name: string): void
  save(): Promise<Endpoint | null>
  discard(): void
}

const endpointKey: InjectionKey<EndpointDraft> = Symbol('endpoint draft')

export function provideEndpointDraft(): EndpointDraft {
  const core = useCoreStore()
  const { formOf, load: loadForms } = useProfileForms()
  const rules = ruleDraft(true)
  const entry = ref<EndpointEntry | null>(null)
  const saved = ref('')
  const { failure, told } = rules
  provideRuleDraft(rules)

  // The list of rules and the rule dialog read the rules in the shape of a preset's, advanced and never built-in.
  function useRules(list: EndpointEntry['rules'], name: string): void {
    rules.draft.value = { id: '', builtin: false, basedOn: '', name, advanced: true, rules: list, dns: '', directDns: '', default: ActionProxy, inherited: [] }
  }
  watch(
    () => entry.value?.profile.name,
    (name) => {
      if (rules.draft.value) rules.draft.value.name = name ?? ''
    },
  )

  const snapshot = () => JSON.stringify([entry.value, rules.draft.value?.rules])
  function take(next: EndpointEntry, savedNow: boolean): void {
    entry.value = { ...next, rules: [] }
    useRules(next.rules, next.profile.name ?? '')
    saved.value = savedNow ? snapshot() : ''
  }

  const value: EndpointDraft = {
    entry,
    rules,
    failure,
    dirty: computed(() => entry.value !== null && snapshot() !== saved.value),
    listsOpen: rules.listsOpen,
    ruleOpen: rules.ruleOpen,
    async load(id) {
      failure.value = ''
      try {
        take(await core.core().call(MethodEndpointsGet, { id }), true)
        return true
      } catch (error) {
        entry.value = null
        rules.draft.value = null
        failure.value = told(error)
        return false
      }
    },
    start(next) {
      failure.value = ''
      take(next, false)
    },
    duplicate(name) {
      const current = entry.value
      if (!current) return
      const copy = JSON.parse(JSON.stringify({ ...current, rules: rules.draft.value?.rules ?? [] })) as EndpointEntry
      value.start({ ...copy, id: '', on: false, profile: { ...copy.profile, name } })
    },
    async save() {
      const current = entry.value
      if (!current || !rules.draft.value) return null
      failure.value = ''
      try {
        await loadForms()
        const profile = cleanProfile(formOf(current.profile.type), current.profile)
        const endpoint = await core.core().call(MethodEndpointsSave, { ...current, profile, rules: rules.draft.value.rules })
        await value.load(endpoint.id)
        return endpoint
      } catch (error) {
        failure.value = told(error)
        return null
      }
    },
    discard() {
      failure.value = ''
      if (!saved.value) {
        entry.value = null
        rules.draft.value = null
        return
      }
      const [kept, list] = JSON.parse(saved.value) as [EndpointEntry, EndpointEntry['rules']]
      take({ ...kept, rules: list }, true)
    },
  }
  provide(endpointKey, value)
  return value
}

export function useEndpointDraft(): EndpointDraft {
  const draft = inject(endpointKey)
  if (!draft) throw new Error('useEndpointDraft needs provideEndpointDraft above it')
  return draft
}
