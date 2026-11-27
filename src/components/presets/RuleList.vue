<script setup lang="ts">
import { ArrowDown, ArrowUp, Lock, MoreHorizontal, Pencil, Plus, Trash2, X } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Button, Input, Menu, MenuItem, MenuSeparator, Select, Switch, vTooltip } from '@/components/ui'
import { useRuleDraft } from '@/composables/usePresetDraft'
import { advancedKinds, inRow, moved, newRule, ruleSummary, simpleKinds } from '@/lib/rules'
import type { InheritedRule, PresetRule } from '@/platform/core-types'
import { ActionEndpoint, ActionProxy, CondPrivate, CondSuffix } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// The rules of a preset or an endpoint, one list checked from the top. A row of one condition and one value changes
// in place; a rule of several conditions changes in the rule dialog. In a preset the rules of the base stand under a
// lock around the user's: the DNS queries Poros answers and the endpoints that are on before them, the local networks
// and, in an advanced preset, the rest after them. The rules of an endpoint have no «Where» and follow the networks its
// server announces.
const props = defineProps<{ advanced: boolean }>()
const { draft, endpoint, apply, checked, ruleOpen, targets: targetsOf } = useRuleDraft()
const core = useCoreStore()
const { t, te } = useI18n()

const locked = computed(() => draft.value?.builtin ?? true)
const kinds = computed(() => (props.advanced ? advancedKinds : simpleKinds).map((value) => ({ value, label: t(`rules.kinds.${value}`) })))
const targets = computed(() => targetsOf())
// In a card narrower than 42rem a row takes two lines: the value goes under the kind and «Where», across the width.
const columns = computed(() => {
  if (endpoint) return 'grid-cols-[36px_160px_minmax(0,1fr)_30px_30px_30px] @max-[42rem]:grid-cols-[36px_minmax(0,1fr)_30px_30px_30px]'
  return props.advanced
    ? 'grid-cols-[36px_160px_minmax(0,1fr)_170px_30px_30px_30px] @max-[42rem]:grid-cols-[36px_minmax(0,1fr)_minmax(0,1fr)_30px_30px_30px]'
    : 'grid-cols-[160px_minmax(0,1fr)_170px_30px_30px_30px] @max-[42rem]:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_30px_30px_30px]'
})
const valueCell = computed(() => (props.advanced ? '@max-[42rem]:order-last @max-[42rem]:col-[2/-1]' : '@max-[42rem]:order-last @max-[42rem]:col-span-full'))

const newKind = ref<string>(CondSuffix)
const newValue = ref('')
const newAction = ref<string>(endpoint ? ActionEndpoint : ActionProxy)
watch(targets, (choices) => {
  if (!endpoint && !choices.some((c) => c.value === newAction.value)) newAction.value = ActionProxy
})
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

function remove(i: number): void {
  const next = rules()
  next.splice(i, 1)
  void apply(next)
}

const label = (rule: InheritedRule) => (te(`presets.rule.${rule.name}`) ? t(`presets.rule.${rule.name}`) : rule.name)
const value = (rule: InheritedRule) => (te(`presets.ruleValue.${rule.name}`) ? t(`presets.ruleValue.${rule.name}`) : '')
const head = computed(() => (props.advanced ? (draft.value?.inherited.filter((r) => r.first) ?? []) : []))
// The endpoints that are on decide before every preset; the simple mode has no section of them.
const endpointNames = computed(() =>
  endpoint
    ? ''
    : core.endpoints
        .filter((e) => e.on && e.need.length === 0)
        .map((e) => e.name)
        .join(', '),
)
const endpointsValue = computed(() =>
  core.settings?.uiMode === 'advanced' ? endpointNames.value : t('rules.endpointsSimple', { names: endpointNames.value }),
)
const tail = computed(() => draft.value?.inherited.filter((r) => !r.first) ?? [])
const restAction = computed(() => (draft.value ? t(`action.${draft.value.default}`) : ''))
const blur = (event: KeyboardEvent) => (event.target as HTMLInputElement).blur()
</script>

<template>
  <SettingsCard v-if="draft" :title="t('rules.title')" class="@container" data-testid="rules">
    <p class="mt-1.5 mb-1 text-xs text-t3">{{ t(endpoint ? 'endpoints.rulesHint' : 'rules.hint') }}</p>
    <!-- Two lines in a narrow card too: the kind and «Where» first, the value and «Add» under them. -->
    <div
      v-if="!locked"
      :class="[
        'grid items-center gap-2 pt-1 pb-2.5',
        endpoint
          ? 'grid-cols-[160px_minmax(0,1fr)_auto] @max-[42rem]:grid-cols-[minmax(0,1fr)_auto]'
          : 'grid-cols-[160px_minmax(0,1fr)_170px_auto] @max-[42rem]:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]',
      ]"
    >
      <Select v-model="newKind" :options="kinds" :class="['h-8 w-full', endpoint && '@max-[42rem]:col-span-2']" :aria-label="t('rules.newKind')" data-testid="rule-new-kind" />
      <Input
        v-model="newValue"
        mono
        :class="['h-8', !endpoint && '@max-[42rem]:col-span-2 @max-[42rem]:row-start-2']"
        :disabled="newKind === CondPrivate"
        :placeholder="newKind === CondPrivate ? t('rules.privateValue') : t(`rules.placeholder.${newKind}`)"
        :aria-label="t('rules.newValue')"
        data-testid="rule-new-value"
        @keydown.enter="add"
      />
      <Select
        v-if="!endpoint"
        v-model="newAction"
        :options="targets"
        class="h-8 w-full @max-[42rem]:col-span-2"
        :aria-label="t('rules.newAction')"
        data-testid="rule-new-action"
      />
      <Button :disabled="adding" data-testid="rule-add" @click="add"><Plus />{{ t('common.add') }}</Button>
    </div>

    <div v-for="rule in head" :key="rule.name" :class="['grid items-center gap-2 border-t border-line py-1.5 text-t3', columns]">
      <span v-if="advanced" />
      <span class="flex items-center gap-1.5"><Lock class="size-3.5" />{{ label(rule) }}</span>
      <span :class="['truncate text-[12.5px] @max-[42rem]:empty:hidden', valueCell]">{{ value(rule) }}</span>
      <span class="pl-[11px] text-[12.5px]">{{ t(`action.${rule.action}`) }}</span>
      <span /><span /><span />
    </div>
    <div v-if="endpointNames" :class="['grid items-center gap-2 border-t border-line py-1.5 text-t3', columns]" data-testid="rule-endpoints">
      <span v-if="advanced" />
      <span class="flex items-center gap-1.5"><Lock class="size-3.5" />{{ t('rules.endpoints') }}</span>
      <span :class="['truncate text-[12.5px]', valueCell]" v-tooltip.cut="endpointsValue">{{ endpointsValue }}</span>
      <span class="truncate pl-[11px] text-[12.5px]">{{ t('action.endpoint') }}</span>
      <span /><span /><span />
    </div>
    <div v-if="endpoint" :class="['grid items-center gap-2 border-t border-line py-1.5 text-t3', columns]" data-testid="rule-networks">
      <span />
      <span class="flex items-center gap-1.5"><Lock class="size-3.5" />{{ t('endpoints.networks') }}</span>
      <span :class="['truncate text-[12.5px]', valueCell]" v-tooltip.cut="t('endpoints.networksValue')">{{ t('endpoints.networksValue') }}</span>
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
        <span v-if="rule.conditions[0]!.kind === CondPrivate" :class="['pl-[11px] text-[12.5px] text-t3', valueCell]">{{ t('rules.privateValue') }}</span>
        <Input
          v-else
          :model-value="rule.conditions[0]!.values[0] ?? ''"
          mono
          :class="['h-8', valueCell]"
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
        <Button variant="ghost" size="sm" :class="['min-w-0 justify-start', valueCell]" v-tooltip="t('rules.editConditions')" @click="ruleOpen = i">
          <span class="truncate">{{ ruleSummary(rule, t) }}</span>
        </Button>
      </template>
      <Select
        v-if="!endpoint"
        :model-value="rule.action"
        :options="targets"
        class="h-8 w-full"
        :aria-label="t('rules.action')"
        data-testid="rule-action"
        @update:model-value="(action: string) => (rule.action = action)"
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

    <div v-for="rule in endpoint ? [] : tail" :key="rule.name" :class="['grid items-center gap-2 border-t border-line py-1.5 text-t3', columns]">
      <span v-if="advanced" />
      <span class="flex items-center gap-1.5"><Lock class="size-3.5" />{{ label(rule) }}</span>
      <span :class="['truncate text-[12.5px] @max-[42rem]:empty:hidden', valueCell]">{{ value(rule) }}</span>
      <span class="pl-[11px] text-[12.5px]">{{ t(`action.${rule.action}`) }}</span>
      <span /><span /><span />
    </div>
    <div v-if="advanced && !endpoint" :class="['grid items-center gap-2 border-t border-line py-1.5 text-t3', columns]" data-testid="rule-rest">
      <span />
      <span class="flex items-center gap-1.5"><Lock class="size-3.5" />{{ t('rules.rest') }}</span>
      <span :class="['truncate text-[12.5px]', valueCell]">{{ t('rules.restValue') }}</span>
      <span class="pl-[11px] text-[12.5px]">{{ restAction }}</span>
      <span /><span /><span />
    </div>
  </SettingsCard>
</template>
