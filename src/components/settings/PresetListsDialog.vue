<script setup lang="ts">
import { TextAlignStart, TriangleAlert, X } from '@lucide/vue'
import { useDebounceFn } from '@vueuse/core'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Banner, Button, Dialog, Tabs, TabsContent, TabsList, TabsTrigger, Textarea } from '@/components/ui'
import { useReason } from '@/composables/useReason'
import type { PresetEntries, PresetListProblem, PresetListsRead } from '@/platform/core-types'
import {
  ActionBlock,
  ActionDirect,
  ActionProxy,
  MethodPresetsReadLists,
  MethodPresetsWriteLists,
  ReasonDuplicate,
} from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// The entries of a preset as a list per action, to paste whole lists at once. The core reads the lines; Apply hands
// the entries to the editor, which saves them with the rest of the preset.
const props = defineProps<{ name: string; entries: PresetEntries }>()
const emit = defineEmits<{ apply: [entries: PresetEntries] }>()
const open = defineModel<boolean>('open', { default: false })

const core = useCoreStore()
const reason = useReason()
const { t } = useI18n()

const tabs = [ActionProxy, ActionDirect, ActionBlock]
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
    lists.value = (await core.core().call(MethodPresetsWriteLists, { sites: props.entries.sites, apps: props.entries.apps })).lists
    await check()
  } catch (error) {
    failure.value = reason(error)
  }
})

async function check(): Promise<PresetListsRead | null> {
  if (!lists.value) return null
  const mine = ++serial
  try {
    const result = await core.core().call(MethodPresetsReadLists, { lists: { ...lists.value } })
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
  emit('apply', { sites: result.sites, apps: result.apps })
  open.value = false
}

const count = (action: string) =>
  read.value ? [...read.value.sites, ...read.value.apps].filter((entry) => entry.action === action).length : 0
const problemsOf = (action: string) => read.value?.problems.filter((problem) => problem.action === action) ?? []
const invalid = computed(() => problemsOf(tab.value).filter((problem) => problem.reason !== ReasonDuplicate))
const duplicates = computed(() => problemsOf(tab.value).filter((problem) => problem.reason === ReasonDuplicate))

// A pasted list can have hundreds of bad lines: the message names the first few.
const named = 5
const short = (text: string) => (text.length > 40 ? `${text.slice(0, 39)}…` : text)

function lineList(problems: PresetListProblem[]): string {
  const lines = problems
    .slice(0, named)
    .map((p) =>
      p.reason === ReasonDuplicate
        ? t('presetLists.lineOn', { line: p.line, text: short(p.text), tab: t(`action.${p.other}`) })
        : t('presetLists.line', { line: p.line, text: short(p.text) }),
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
            <template v-if="problemsOf(action).length > 0">
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
        <p class="text-xs text-t3">{{ t('presetLists.hint') }}</p>
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
