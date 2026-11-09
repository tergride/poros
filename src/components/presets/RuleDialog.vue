<script setup lang="ts">
import { ChevronDown, Plus, TriangleAlert, X } from '@lucide/vue'
import { useDebounceFn } from '@vueuse/core'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Checkbox, Dialog, Input, Menu, MenuItem, Segmented, Select, Switch, Textarea } from '@/components/ui'
import { usePresetDraft } from '@/composables/usePresetDraft'
import { useFailure } from '@/composables/useReason'
import { advancedKinds, problemText, protocols, ruleActions } from '@/lib/rules'
import { isCoreError } from '@/platform'
import type { PresetRule } from '@/platform/core-types'
import { ActionDirect, CondIPVersion, CondNetwork, CondPrivate, CondProtocol, MethodPresetsCheck } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// A rule of an advanced preset: every condition must match, any value of one does. Each kind comes once, so «Condition»
// offers the kinds the rule has not got. The core checks the rule as it is typed and names the condition and line of
// a wrong value; Done puts it into the list, where narrower rules rise over wider ones.
const core = useCoreStore()
const { draft, ruleOpen } = usePresetDraft()
const failure = useFailure()
const { t } = useI18n()

interface Condition {
  kind: string
  // The values one per line; a network or an IP version is one value.
  text: string
}
const name = ref('')
const conditions = ref<Condition[]>([])
const invert = ref(false)
const action = ref<string>(ActionDirect)
const problem = ref('')
const busy = ref(false)

const open = computed({
  get: () => ruleOpen.value !== null,
  set: (shown: boolean) => {
    if (!shown) ruleOpen.value = null
  },
})

const first = (kind: string) => (kind === CondNetwork ? 'udp' : kind === CondIPVersion ? '4' : '')

watch(ruleOpen, (i) => {
  if (i === null || !draft.value) return
  const rule = i >= 0 ? draft.value.rules[i] : undefined
  name.value = rule?.name ?? ''
  conditions.value = rule ? rule.conditions.map((c) => ({ kind: c.kind, text: c.values.join('\n') })) : [{ kind: advancedKinds[0]!, text: '' }]
  invert.value = rule?.invert ?? false
  action.value = rule?.action ?? ActionDirect
  problem.value = ''
})

const kindLabel = (kind: string) => t(kind === 'suffix' ? 'rules.dialog.suffix' : `rules.kinds.${kind}`)
const free = computed(() => advancedKinds.filter((kind) => !conditions.value.some((c) => c.kind === kind)))
const kindsFor = (own: string) => advancedKinds.filter((kind) => kind === own || free.value.includes(kind)).map((value) => ({ value, label: kindLabel(value) }))
const actions = computed(() => ruleActions.map((value) => ({ value, label: t(`action.${value}`) })))

function addCondition(kind: string): void {
  conditions.value.push({ kind, text: first(kind) })
}

function setKind(condition: Condition, kind: string): void {
  condition.kind = kind
  condition.text = first(kind)
}

function toggle(condition: Condition, protocol: string, on: boolean): void {
  const chosen = condition.text.split('\n').filter((p) => p && p !== protocol)
  condition.text = (on ? [...chosen, protocol] : chosen).join('\n')
}

function built(): PresetRule {
  return {
    name: name.value.trim(),
    conditions: conditions.value.map((c) => ({
      kind: c.kind,
      values: c.kind === CondPrivate ? [] : c.text.split('\n').map((v) => v.trim()).filter(Boolean),
    })),
    invert: invert.value,
    off: ruleOpen.value !== null && ruleOpen.value >= 0 ? (draft.value?.rules[ruleOpen.value]?.off ?? false) : false,
    action: action.value,
  }
}

// What the core says about the rules, the rule's own problem named by its condition and line.
async function check(rules: PresetRule[]): Promise<PresetRule[] | null> {
  try {
    const result = await core.core().call(MethodPresetsCheck, { advanced: true, rules })
    problem.value = ''
    return result.rules
  } catch (error) {
    problem.value = isCoreError(error) && typeof error.data?.rule === 'number' ? problemText(error.data, t) : failure(error)
    return null
  }
}

const checkSoon = useDebounceFn(() => (open.value && conditions.value.length > 0 ? check([built()]) : null), 250)
watch([conditions, name], () => void checkSoon(), { deep: true })

async function done(): Promise<void> {
  const i = ruleOpen.value
  if (!draft.value || i === null || busy.value) return
  busy.value = true
  try {
    const rules = [...draft.value.rules]
    if (i >= 0) rules[i] = built()
    else rules.push(built())
    const result = await check(rules)
    if (!result || !draft.value) return
    draft.value.rules = result
    ruleOpen.value = null
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('rules.dialog.title')" class="w-[720px]">
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="open = false"><X /></Button>
    </template>
    <div class="grid gap-3">
      <label class="grid max-w-[360px] gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('rules.dialog.name') }}</span>
        <Input v-model="name" :placeholder="t('rules.dialog.namePlaceholder')" data-testid="rule-name" />
      </label>
      <div class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('rules.dialog.conditions') }}</span>
        <div class="grid gap-2">
          <div v-for="(condition, i) in conditions" :key="i" class="grid grid-cols-[200px_1fr_30px] items-start gap-2" data-testid="rule-condition">
            <Select
              :model-value="condition.kind"
              :options="kindsFor(condition.kind)"
              class="w-full"
              :aria-label="t('rules.dialog.kind')"
              @update:model-value="(kind: string) => setKind(condition, kind)"
            />
            <div v-if="condition.kind === CondNetwork">
              <Segmented v-model="condition.text" :options="[{ value: 'tcp', label: 'TCP' }, { value: 'udp', label: 'UDP' }]" :aria-label="kindLabel(condition.kind)" />
            </div>
            <div v-else-if="condition.kind === CondIPVersion">
              <Segmented v-model="condition.text" :options="[{ value: '4', label: 'IPv4' }, { value: '6', label: 'IPv6' }]" :aria-label="kindLabel(condition.kind)" />
            </div>
            <div v-else-if="condition.kind === CondProtocol" class="flex flex-wrap gap-x-3.5 gap-y-1.5 pt-1.5">
              <label v-for="protocol in protocols" :key="protocol" class="flex items-center gap-1.5 text-[12.5px]">
                <Checkbox :model-value="condition.text.split('\n').includes(protocol)" @update:model-value="(on: boolean) => toggle(condition, protocol, on)" />
                {{ protocol === 'bittorrent' ? 'BitTorrent' : protocol.toUpperCase() }}
              </label>
            </div>
            <span v-else-if="condition.kind === CondPrivate" class="pt-2 text-[12.5px] text-t3">{{ t('rules.privateValue') }}</span>
            <Textarea
              v-else
              v-model="condition.text"
              mono
              rows="2"
              class="min-h-0 resize-y"
              :placeholder="t(`rules.placeholder.${condition.kind}`)"
              :aria-label="kindLabel(condition.kind)"
              data-testid="rule-values"
            />
            <Button variant="ghost" size="icon-sm" :aria-label="t('rules.dialog.remove')" @click="conditions.splice(i, 1)"><X /></Button>
          </div>
        </div>
        <div class="mt-0.5 flex items-center gap-2.5">
          <Menu min-width="260px">
            <template #trigger>
              <Button size="sm" :disabled="free.length === 0" data-testid="rule-add-condition"><Plus />{{ t('rules.dialog.addCondition') }}<ChevronDown class="size-3.5" /></Button>
            </template>
            <MenuItem v-for="kind in free" :key="kind" @select="addCondition(kind)">{{ kindLabel(kind) }}</MenuItem>
          </Menu>
          <span class="flex-1 text-xs text-t3">{{ t('rules.dialog.conditionsHint') }}</span>
        </div>
      </div>
      <div v-if="problem" class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-2.5" role="alert" data-testid="rule-problem">
        <TriangleAlert class="size-4 flex-none text-warn" /><span>{{ problem }}</span>
      </div>
      <label class="flex items-start gap-3">
        <Switch v-model="invert" :aria-label="t('rules.dialog.invert')" />
        <span>{{ t('rules.dialog.invert') }}<br /><span class="text-xs text-t3">{{ t('rules.dialog.invertHint') }}</span></span>
      </label>
      <label class="grid max-w-[360px] gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('rules.action') }}</span>
        <Select v-model="action" :options="actions" class="w-full" :aria-label="t('rules.action')" />
      </label>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="busy || !!problem || conditions.length === 0" data-testid="rule-done" @click="done">{{ t('rules.dialog.done') }}</Button>
    </template>
  </Dialog>
</template>
