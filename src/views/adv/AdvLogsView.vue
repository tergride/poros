<script setup lang="ts">
import { Check, Copy, Search, Trash2 } from '@lucide/vue'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useTemplateRef } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Input, Select } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { filterLog, logLevel } from '@/lib/logs'
import { MethodLogsGet } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// The log screen of the advanced mode: the end of the core's log, new lines once a second while it is open, with a
// filter by text or a regular expression and by level. «Clear» empties the screen, not the file.
const core = useCoreStore()
const { t } = useI18n()
const attempt = useAttempt()
const all = ref<string[]>([])
const text = ref('')
const matchCase = ref(false)
const regex = ref(false)
const level = ref('all')
const copied = ref(false)
const box = useTemplateRef<HTMLElement>('box')
let next = 0
let timer: ReturnType<typeof setInterval> | undefined

const levels = computed(() =>
  ['all', 'error', 'warn', 'info'].map((value) => ({ value, label: t(`adv.logs.level.${value}`) })),
)
const filtered = computed(() => filterLog(all.value, { text: text.value, matchCase: matchCase.value, regex: regex.value, level: level.value }))

const tone: Record<string, string> = { error: 'text-bad', warn: 'text-warn', info: 'text-t2', debug: 'text-t3' }

async function read(): Promise<void> {
  const chunk = await core.core().call(MethodLogsGet, { after: next })
  next = chunk.next
  if (!chunk.reset && chunk.lines.length === 0) return
  const el = box.value
  const atEnd = !el || el.scrollHeight - el.scrollTop - el.clientHeight < 24
  const keep = core.settings?.logLines ?? 500
  all.value = (chunk.reset ? chunk.lines : [...all.value, ...chunk.lines]).slice(-keep)
  if (atEnd) {
    await nextTick()
    if (box.value) box.value.scrollTop = box.value.scrollHeight
  }
}

async function copy(): Promise<void> {
  await navigator.clipboard.writeText(filtered.value.lines.join('\n'))
  copied.value = true
  setTimeout(() => (copied.value = false), 2000)
}

onMounted(() => {
  void attempt(read)
  timer = setInterval(() => void read().catch(() => undefined), 1000)
})
onBeforeUnmount(() => clearInterval(timer))
</script>

<template>
  <section class="flex min-h-0 min-w-0 flex-1 flex-col">
    <div class="flex flex-none items-center gap-2 border-b border-line px-3.5 py-2.5">
      <div class="relative min-w-0 flex-1">
        <Search class="absolute top-[7px] left-2.5 size-4 text-t3" />
        <Input v-model="text" class="h-[30px] pl-8" :placeholder="t('adv.logs.filter')" :aria-label="t('adv.logs.filter')" data-testid="log-filter" />
      </div>
      <Button
        size="sm"
        :class="matchCase && 'border-blue bg-s4'"
        :aria-pressed="matchCase"
        :title="t('adv.logs.matchCase')"
        :aria-label="t('adv.logs.matchCase')"
        data-testid="log-case"
        @click="matchCase = !matchCase"
      >Aa</Button>
      <Button
        size="sm"
        :class="['font-mono', regex && 'border-blue bg-s4']"
        :aria-pressed="regex"
        :title="t('adv.logs.regex')"
        :aria-label="t('adv.logs.regex')"
        data-testid="log-regex"
        @click="regex = !regex"
      >.*</Button>
      <Select v-model="level" :options="levels" class="h-[30px] w-[150px]" :aria-label="t('adv.logs.levelLabel')" data-testid="log-level" />
      <Button size="sm" :disabled="filtered.lines.length === 0" data-testid="log-copy" @click="attempt(copy)">
        <Check v-if="copied" /><Copy v-else />{{ t(copied ? 'adv.logs.copied' : 'adv.logs.copy') }}
      </Button>
      <Button size="sm" data-testid="log-clear" @click="all = []"><Trash2 />{{ t('adv.logs.clear') }}</Button>
    </div>
    <p v-if="filtered.invalid" class="px-3.5 pt-2 text-xs text-warn">{{ t('adv.logs.badRegex') }}</p>
    <div ref="box" class="min-h-0 flex-1 overflow-auto px-3.5 py-2.5 font-mono text-[12.5px] leading-relaxed select-text" data-testid="log-lines">
      <p v-if="filtered.lines.length === 0" class="text-t3">{{ t(all.length ? 'adv.logs.nothing' : 'adv.logs.empty') }}</p>
      <div v-for="(line, i) in filtered.lines" :key="i" :class="['break-all whitespace-pre-wrap', tone[logLevel(line)]]">{{ line }}</div>
    </div>
  </section>
</template>
