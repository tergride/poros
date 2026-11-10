<script setup lang="ts">
import { Copy, ScrollText, X } from '@lucide/vue'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog } from '@/components/ui'
import { MethodLogsGet } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

const open = defineModel<boolean>('open', { default: false })
const core = useCoreStore()
const { t } = useI18n()
const lines = ref<string[]>([])
const copied = ref(false)

watch(open, async (shown) => {
  copied.value = false
  if (shown) lines.value = (await core.core().call(MethodLogsGet, { after: 0 })).lines
})

async function copy(): Promise<void> {
  await navigator.clipboard.writeText(lines.value.join('\n'))
  copied.value = true
}

function level(line: string): string {
  if (/\b(ERROR|FATAL|panic)\b/.test(line)) return 'text-bad'
  if (/\bWARN(ING)?\b/.test(line)) return 'text-warn'
  return 'text-t2'
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('logs.title')" class="h-[680px] w-[900px]">
    <template #icon><ScrollText /></template>
    <template #actions>
      <Button size="sm" :disabled="lines.length === 0" @click="copy"><Copy />{{ copied ? t('logs.copied') : t('logs.copy') }}</Button>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="open = false"><X /></Button>
    </template>
    <div class="h-full overflow-auto rounded-[10px] border border-line bg-s0 p-3 font-mono text-xs leading-relaxed select-text">
      <p v-if="lines.length === 0" class="text-t3">{{ t('logs.empty') }}</p>
      <div v-for="(line, i) in lines" :key="i" :class="['break-all whitespace-pre-wrap', level(line)]">{{ line }}</div>
    </div>
  </Dialog>
</template>
