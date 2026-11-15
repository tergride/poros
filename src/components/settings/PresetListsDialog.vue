<script setup lang="ts">
import { CircleQuestionMark, TextAlignStart, TriangleAlert, X } from '@lucide/vue'
import { useDebounceFn } from '@vueuse/core'
import { computed, ref, watch } from 'vue'
import { I18nT, useI18n } from 'vue-i18n'

import { Banner, Button, Dialog, Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui'
import PresetListsHelp from '@/components/settings/PresetListsHelp.vue'
import PresetListsText from '@/components/settings/PresetListsText.vue'
import { useFailure } from '@/composables/useReason'
import { kindOfLine, listKinds } from '@/lib/rules'
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

// The rules of a preset as a list per action, to paste whole lists at once: every line starts with the prefix of its
// kind. The core reads the lines; Apply hands the rules to the editor, which saves them with the rest of the preset.
// Rules of several conditions and the locked ones stay out of the window and in place.
const props = defineProps<{ name: string; advanced: boolean; rules: PresetRule[] }>()
const emit = defineEmits<{ apply: [rules: PresetRule[]] }>()
const open = defineModel<boolean>('open', { default: false })

const core = useCoreStore()
const reason = useFailure()
const { t } = useI18n()

const tabs = computed(() => (props.advanced ? [ActionProxy, ActionDirect, ActionBlock, ActionWARP] : [ActionProxy, ActionDirect, ActionBlock]))
const kinds = computed(() => listKinds(props.advanced))
const tab = ref<string>(ActionProxy)
const helpOpen = ref(false)
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
const invalidText = computed(() =>
  invalid.value.length > 0 ? t('presetLists.invalid', { n: invalid.value.length, lines: lineList(invalid.value) }, invalid.value.length) : '',
)
const duplicateText = computed(() => (duplicates.value.length > 0 ? t('presetLists.duplicate', { lines: lineList(duplicates.value) }) : ''))
const problemText = computed(() => [invalidText.value, duplicateText.value].filter(Boolean).join(' '))

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
          <PresetListsText
            v-model="lists[action]"
            :kinds="kinds"
            :aria-label="t(`action.${action}`)"
            data-testid="lists-text"
            @update:model-value="checkSoon"
          />
        </template>
        <Banner v-if="failure">{{ failure }}</Banner>
        <!-- The problems take the place of the hint in two lines at most, so that the field keeps its height while typing. -->
        <div class="flex min-h-8 items-center gap-3">
          <div v-if="problemText" class="flex min-w-0 flex-1 items-start gap-2 text-xs leading-4" role="alert">
            <TriangleAlert class="size-4 flex-none text-warn" />
            <p class="line-clamp-2" :title="problemText">
              <span v-if="invalidText" data-testid="lists-invalid">{{ invalidText }}</span>
              {{ ' ' }}
              <span v-if="duplicateText" data-testid="lists-duplicate">{{ duplicateText }}</span>
            </p>
          </div>
          <I18nT v-else keypath="presetLists.hint" tag="p" scope="global" class="flex-1 text-xs text-t3">
            <template #example><code class="font-mono text-t2">suffix:example.com</code></template>
          </I18nT>
          <Button size="sm" data-testid="lists-help" @click="helpOpen = true"><CircleQuestionMark />{{ t('presetLists.help.open') }}</Button>
        </div>
      </TabsContent>
      <template #footer>
        <span class="sp text-xs text-t3">{{ t('presetLists.locked') }}</span>
        <Button variant="ghost" @click="open = false">{{ t('common.cancel') }}</Button>
        <Button variant="primary" :disabled="!read || read.problems.length > 0" data-testid="lists-apply" @click="apply">
          {{ t('presetLists.apply') }}
        </Button>
      </template>
    </Dialog>
    <PresetListsHelp v-model:open="helpOpen" :advanced="advanced" />
  </Tabs>
</template>
