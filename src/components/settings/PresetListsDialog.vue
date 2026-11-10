<script setup lang="ts">
import { TextAlignStart, TriangleAlert, X } from '@lucide/vue'
import { useDebounceFn } from '@vueuse/core'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Banner, Button, Dialog, Tabs, TabsContent, TabsList, TabsTrigger, Textarea } from '@/components/ui'
import { useFailure } from '@/composables/useReason'
import { kindOfLine } from '@/lib/rules'
import type { PresetListProblem, PresetListsRead, PresetRule } from '@/platform/core-types'
import {
  ActionBlock,
  ActionDirect,
  ActionProxy,
  ActionWARP,
  MethodPresetsReadLists,
  MethodPresetsWriteLists,
  ReasonAdvancedOnly,
  ReasonDuplicate,
  ReasonNoPrefix,
} from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// The rules of a preset as a list per action, as Throne's simple rules, to paste whole lists at once: every line
// starts with the prefix of its kind. The core reads the lines; Apply hands the rules to the editor, which saves them
// with the rest of the preset. Rules of several conditions and the locked ones stay out of the window and in place.
const props = defineProps<{ name: string; advanced: boolean; rules: PresetRule[] }>()
const emit = defineEmits<{ apply: [rules: PresetRule[]] }>()
const open = defineModel<boolean>('open', { default: false })

const core = useCoreStore()
const reason = useFailure()
const { t } = useI18n()

const tabs = computed(() => (props.advanced ? [ActionProxy, ActionDirect, ActionBlock, ActionWARP] : [ActionProxy, ActionDirect, ActionBlock]))
const tab = ref<string>(ActionProxy)
const lists = ref<Record<string, string> | null>(null)
const read = ref<PresetListsRead | null>(null)
const failure = ref('')
// Only the answer to the latest text counts.
let serial = 0

watch(open, async (shown) => {
  if (!shown) return
  tab.value = ActionProxy
  lists.value = null
  read.value = null
  failure.value = ''
  try {
    lists.value = (await core.core().call(MethodPresetsWriteLists, { advanced: props.advanced, rules: props.rules })).lists
    await check()
  } catch (error) {
    failure.value = reason(error)
  }
})

async function check(): Promise<PresetListsRead | null> {
  if (!lists.value) return null
  const mine = ++serial
  try {
    const result = await core.core().call(MethodPresetsReadLists, { advanced: props.advanced, rules: props.rules, lists: { ...lists.value } })
    if (mine === serial) {
      read.value = result
      failure.value = ''
    }
    return result
  } catch (error) {
    failure.value = reason(error)
    return null
  }
}

const checkSoon = useDebounceFn(check, 250)

async function apply(): Promise<void> {
  const result = await check()
  if (!result || result.problems.length > 0) return
  emit('apply', result.rules)
  open.value = false
}

// The lines of a tab: a list of hundreds counts what is written, not what the core has read so far.
const count = (action: string) => (lists.value?.[action] ?? '').split('\n').filter((line) => line.trim() && !line.trim().startsWith('#')).length
const problemsOf = (action: string) => read.value?.problems.filter((problem) => problem.reason !== ReasonDuplicate && problem.action === action) ?? []
const duplicatesOf = (action: string) => read.value?.problems.filter((problem) => problem.reason === ReasonDuplicate && problem.action === action) ?? []
const invalid = computed(() => problemsOf(tab.value))
const duplicates = computed(() => duplicatesOf(tab.value))

// A pasted list can have hundreds of bad lines: the message names the first few.
const named = 5
const short = (text: string) => (text.length > 40 ? `${text.slice(0, 39)}…` : text)

function why(problem: PresetListProblem): string {
  if (problem.reason === ReasonNoPrefix) return t('presetLists.noPrefix')
  if (problem.reason === ReasonAdvancedOnly) return t('presetLists.advancedOnly')
  const kind = kindOfLine(problem.text)
  return kind ? t(`rules.invalid.${kind}`) : t('rules.problem.invalid')
}

function lineList(problems: PresetListProblem[]): string {
  const lines = problems
    .slice(0, named)
    .map((p) =>
      p.reason === ReasonDuplicate
        ? t('presetLists.lineOn', { line: p.line, text: short(p.text), tab: t(`action.${p.other}`) })
        : t('presetLists.line', { line: p.line, text: short(p.text), why: why(p) }),
    )
    .join(', ')
  return problems.length > named ? t('presetLists.more', { lines, n: problems.length - named }) : lines
}

const title = computed(() =>
  props.name.trim() ? t('presetLists.title', { name: props.name.trim() }) : t('presetLists.titleUnnamed'),
)
</script>

<template>
  <Tabs v-model="tab">
    <Dialog v-model:open="open" :title="title" class="h-[600px] w-[720px]">
      <template #icon><TextAlignStart /></template>
      <template #actions>
        <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="open = false"><X /></Button>
      </template>
      <template #tabs>
        <TabsList class="px-3">
          <TabsTrigger v-for="action in tabs" :key="action" :value="action" :data-testid="`lists-tab-${action}`">
            {{ t(`action.${action}`) }} · {{ count(action) }}
            <template v-if="problemsOf(action).length + duplicatesOf(action).length > 0">
              <TriangleAlert class="text-warn" /><span class="sr-only">{{ t('presetLists.problems') }}</span>
            </template>
          </TabsTrigger>
        </TabsList>
      </template>
      <TabsContent v-for="action in tabs" :key="action" :value="action" class="flex h-full flex-col gap-2.5 pt-2.5">
        <template v-if="lists">
          <Textarea
            v-model="lists[action]"
            mono
            class="min-h-0 flex-1 resize-none"
            spellcheck="false"
            :aria-label="t(`action.${action}`)"
            data-testid="lists-text"
            @update:model-value="checkSoon"
          />
        </template>
        <div
          v-if="invalid.length > 0"
          class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3"
          role="alert"
          data-testid="lists-invalid"
        >
          <TriangleAlert class="size-4 flex-none text-warn" />
          <span>{{ t('presetLists.invalid', { n: invalid.length, lines: lineList(invalid) }, invalid.length) }}</span>
        </div>
        <div
          v-if="duplicates.length > 0"
          class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3"
          role="alert"
          data-testid="lists-duplicate"
        >
          <TriangleAlert class="size-4 flex-none text-warn" />
          <span>{{ t('presetLists.duplicate', { lines: lineList(duplicates) }) }}</span>
        </div>
        <Banner v-if="failure">{{ failure }}</Banner>
        <p class="text-xs text-t3">{{ t(advanced ? 'presetLists.hintAdvanced' : 'presetLists.hint') }}</p>
      </TabsContent>
      <template #footer>
        <span class="sp text-xs text-t3">{{ t('presetLists.locked') }}</span>
        <Button variant="ghost" @click="open = false">{{ t('common.cancel') }}</Button>
        <Button variant="primary" :disabled="!read || read.problems.length > 0" data-testid="lists-apply" @click="apply">
          {{ t('presetLists.apply') }}
        </Button>
      </template>
    </Dialog>
  </Tabs>
</template>
