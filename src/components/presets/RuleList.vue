<script setup lang="ts">
import { ArrowDown, ArrowUp, Lock, MoreHorizontal, Pencil, Plus, Trash2, X } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Button, Input, Menu, MenuItem, MenuSeparator, Select, Switch } from '@/components/ui'
import { usePresetDraft } from '@/composables/usePresetDraft'
import { advancedKinds, inRow, moved, newRule, ruleActions, ruleSummary, simpleKinds } from '@/lib/rules'
import type { InheritedRule, PresetRule } from '@/platform/core-types'
import { ActionProxy, CondPrivate, CondSuffix } from '@/platform/core-types'

// The rules of a preset, one list checked from the top. A row of one condition and one value changes in place; a
// rule of several conditions changes in the rule dialog. The rules of the base stand under a lock around the user's:
// the DNS queries Poros answers before them, the local networks and, in an advanced preset, the rest after them.
const props = defineProps<{ advanced: boolean }>()
const { draft, apply, checked, ruleOpen } = usePresetDraft()
const { t, te } = useI18n()

const locked = computed(() => draft.value?.builtin ?? true)
const kinds = computed(() => (props.advanced ? advancedKinds : simpleKinds).map((value) => ({ value, label: t(`rules.kinds.${value}`) })))
const actions = computed(() => ruleActions.map((value) => ({ value, label: t(`action.${value}`) })))
const columns = computed(() =>
  props.advanced ? 'grid-cols-[36px_160px_minmax(0,1fr)_170px_30px_30px_30px]' : 'grid-cols-[160px_minmax(0,1fr)_170px_30px_30px_30px]',
)

const newKind = ref<string>(CondSuffix)
const newValue = ref('')
const newAction = ref<string>(ActionProxy)
const adding = ref(false)

async function add(): Promise<void> {
  const value = newValue.value.trim()
  if (!draft.value || adding.value || (!value && newKind.value !== CondPrivate)) return
  adding.value = true
  try {
    const rules = await checked([...draft.value.rules, newRule(newKind.value, value, newAction.value)])
    if (rules && draft.value) {
      draft.value.rules = rules
      newValue.value = ''
    }
  } finally {
    adding.value = false
  }
}

const rules = () => [...(draft.value?.rules ?? [])]

function replace(i: number, rule: PresetRule): Promise<boolean> {
  const next = rules()
  next[i] = rule
  return apply(next)
}

function setKind(rule: PresetRule, i: number, kind: string): void {
  const values = kind === CondPrivate ? [] : (rule.conditions[0]?.values ?? [])
  void replace(i, { ...rule, conditions: [{ kind, values }] })
}

function setValue(rule: PresetRule, i: number, event: Event): void {
  const value = (event.target as HTMLInputElement).value.trim()
  if (value === (rule.conditions[0]?.values[0] ?? '')) return
  void replace(i, { ...rule, conditions: [{ kind: rule.conditions[0]?.kind ?? CondSuffix, values: value ? [value] : [] }] })
}

function setAction(rule: PresetRule, action: string): void {
  rule.action = action
}

function remove(i: number): void {
  const next = rules()
  next.splice(i, 1)
  void apply(next)
}

const label = (rule: InheritedRule) => (te(`presets.rule.${rule.name}`) ? t(`presets.rule.${rule.name}`) : rule.name)
const value = (rule: InheritedRule) => (te(`presets.ruleValue.${rule.name}`) ? t(`presets.ruleValue.${rule.name}`) : '')
const head = computed(() => (props.advanced ? (draft.value?.inherited.filter((r) => r.first) ?? []) : []))
const tail = computed(() => draft.value?.inherited.filter((r) => !r.first) ?? [])
const restAction = computed(() => (draft.value ? t(`action.${draft.value.default}`) : ''))
const blur = (event: KeyboardEvent) => (event.target as HTMLInputElement).blur()
</script>

<template>
  <SettingsCard v-if="draft" :title="t('rules.title')" data-testid="rules">
    <p class="mt-1.5 mb-1 text-xs text-t3">{{ t('rules.hint') }}</p>
    <div v-if="!locked" class="grid grid-cols-[160px_minmax(0,1fr)_170px_auto] items-center gap-2 pt-1 pb-2.5">
      <Select v-model="newKind" :options="kinds" class="h-8 w-full" :aria-label="t('rules.newKind')" data-testid="rule-new-kind" />
      <Input
        v-model="newValue"
        mono
        class="h-8"
        :disabled="newKind === CondPrivate"
        :placeholder="newKind === CondPrivate ? t('rules.privateValue') : t(`rules.placeholder.${newKind}`)"
        :aria-label="t('rules.newValue')"
        data-testid="rule-new-value"
        @keydown.enter="add"
      />
      <Select v-model="newAction" :options="actions" class="h-8 w-full" :aria-label="t('rules.newAction')" />
      <Button :disabled="adding" data-testid="rule-add" @click="add"><Plus />{{ t('common.add') }}</Button>
    </div>

    <div v-for="rule in head" :key="rule.name" :class="['grid items-center gap-2 border-t border-line py-1.5 text-t3', columns]">
      <span v-if="advanced" />
      <span class="flex items-center gap-1.5"><Lock class="size-3.5" />{{ label(rule) }}</span>
      <span class="truncate text-[12.5px]">{{ value(rule) }}</span>
      <span class="pl-[11px] text-[12.5px]">{{ t(`action.${rule.action}`) }}</span>
      <span /><span /><span />
    </div>

    <div
      v-for="(rule, i) in draft.rules"
      :key="`${i}:${JSON.stringify(rule)}`"
      :class="['grid items-center gap-2 border-t border-line py-1.5', columns, advanced && rule.off && '[&>*:not(:first-child)]:opacity-50']"
      data-testid="rule-row"
    >
      <span v-if="advanced">
        <Switch :model-value="!rule.off" size="sm" :aria-label="t('rules.on')" @update:model-value="(on: boolean) => (rule.off = !on)" />
      </span>
      <template v-if="inRow(rule)">
        <Select
          :model-value="rule.conditions[0]!.kind"
          :options="kinds"
          class="h-8 w-full"
          :aria-label="t('rules.kind')"
          @update:model-value="(kind: string) => setKind(rule, i, kind)"
        />
        <span v-if="rule.conditions[0]!.kind === CondPrivate" class="pl-[11px] text-[12.5px] text-t3">{{ t('rules.privateValue') }}</span>
        <Input
          v-else
          :model-value="rule.conditions[0]!.values[0] ?? ''"
          mono
          class="h-8"
          :aria-label="t('rules.value')"
          data-testid="rule-value"
          @change="(event: Event) => setValue(rule, i, event)"
          @keydown.enter="blur"
        />
      </template>
      <template v-else>
        <span class="inline-flex h-[22px] items-center justify-self-start rounded-md bg-s4 px-2 text-[11.5px] text-t2">
          {{ t(rule.conditions.length > 1 ? 'rules.several' : 'rules.inverted') }}
        </span>
        <Button variant="ghost" size="sm" class="min-w-0 justify-start" :title="t('rules.editConditions')" @click="ruleOpen = i">
          <span class="truncate">{{ ruleSummary(rule, t) }}</span>
        </Button>
      </template>
      <Select
        :model-value="rule.action"
        :options="actions"
        class="h-8 w-full"
        :aria-label="t('rules.action')"
        @update:model-value="(action: string) => setAction(rule, action)"
      />
      <Button variant="ghost" size="icon-sm" :disabled="i === 0" :aria-label="t('rules.up')" @click="apply(moved(draft.rules, i, -1))"><ArrowUp /></Button>
      <Button variant="ghost" size="icon-sm" :disabled="i === draft.rules.length - 1" :aria-label="t('rules.down')" @click="apply(moved(draft.rules, i, 1))">
        <ArrowDown />
      </Button>
      <Menu v-if="advanced" align="end" min-width="220px">
        <template #trigger>
          <Button variant="ghost" size="icon-sm" :aria-label="t('common.more')" data-testid="rule-menu"><MoreHorizontal /></Button>
        </template>
        <MenuItem @select="ruleOpen = i"><Pencil />{{ t('rules.editConditions') }}</MenuItem>
        <MenuSeparator />
        <MenuItem danger @select="remove(i)"><Trash2 />{{ t('common.remove') }}</MenuItem>
      </Menu>
      <Button v-else variant="ghost" size="icon-sm" :aria-label="t('common.remove')" @click="remove(i)"><X /></Button>
    </div>

    <div v-for="rule in tail" :key="rule.name" :class="['grid items-center gap-2 border-t border-line py-1.5 text-t3', columns]">
      <span v-if="advanced" />
      <span class="flex items-center gap-1.5"><Lock class="size-3.5" />{{ label(rule) }}</span>
      <span class="truncate text-[12.5px]">{{ value(rule) }}</span>
      <span class="pl-[11px] text-[12.5px]">{{ t(`action.${rule.action}`) }}</span>
      <span /><span /><span />
    </div>
    <div v-if="advanced" :class="['grid items-center gap-2 border-t border-line py-1.5 text-t3', columns]" data-testid="rule-rest">
      <span />
      <span class="flex items-center gap-1.5"><Lock class="size-3.5" />{{ t('rules.rest') }}</span>
      <span class="truncate text-[12.5px]">{{ t('rules.restValue') }}</span>
      <span class="pl-[11px] text-[12.5px]">{{ restAction }}</span>
      <span /><span /><span />
    </div>
  </SettingsCard>
</template>
