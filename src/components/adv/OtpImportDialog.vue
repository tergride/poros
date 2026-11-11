<script setup lang="ts">
import { Download, FileUp } from '@lucide/vue'
import { watchDebounced } from '@vueuse/core'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog, Textarea } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useOtpProblem } from '@/composables/useOtpProblem'
import { MethodOTPImport, MethodOTPParse, type OTPParsed } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// Codes pasted or read from a file, one per line: the core reads them as they come and names a line it cannot take,
// and «Import» adds the rest.
const open = defineModel<boolean>('open', { default: false })
const core = useCoreStore()
const ui = useUiStore()
const attempt = useAttempt()
const problemText = useOtpProblem()
const { t } = useI18n()

const text = ref('')
const parsed = ref<OTPParsed | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
watch(open, () => {
  text.value = ''
  parsed.value = null
})

watchDebounced(
  text,
  async (value) => {
    parsed.value = value.trim() ? await core.core().call(MethodOTPParse, { text: value }) : null
  },
  { debounce: 150 },
)

const state = computed(() => {
  const p = parsed.value
  if (!p) return ''
  const found = t('adv.tools.found', p.entries.length)
  if (p.problems.length === 0) return found
  const lines = p.problems.slice(0, 3).map(problemText).join(' ')
  return `${found} ${lines}${p.problems.length > 3 ? ` ${t('adv.tools.moreProblems', p.problems.length - 3)}` : ''}`
})

async function fromFile(): Promise<void> {
  const file = fileInput.value?.files?.[0]
  if (file) text.value = await file.text()
  if (fileInput.value) fileInput.value.value = ''
}

async function importAll(): Promise<void> {
  await attempt(async () => {
    const result = await core.core().call(MethodOTPImport, { text: text.value })
    open.value = false
    const added = t('adv.tools.added', result.added)
    ui.notice = { tone: 'info', text: result.duplicates ? `${added} ${t('adv.tools.duplicates', result.duplicates)}` : added }
  })
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('adv.tools.importTitle')" class="w-[720px]">
    <template #icon><Download /></template>
    <div class="grid gap-2.5">
      <Textarea
        v-model="text"
        mono
        class="h-[180px] resize-none"
        spellcheck="false"
        :placeholder="t('adv.tools.importPlaceholder')"
        :aria-label="t('adv.tools.importTitle')"
        data-testid="otp-import-text"
      />
      <p
        v-if="state"
        :class="['rounded-[10px] border border-line bg-s1 px-3.5 py-2.5 text-[13px]', parsed?.problems.length ? 'text-warn' : 'text-t1']"
        data-testid="otp-import-state"
      >
        {{ state }}
      </p>
      <p class="text-xs text-t3">{{ t('adv.tools.importHint') }}</p>
      <input ref="fileInput" type="file" class="hidden" accept=".txt,.json" @change="fromFile" />
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="fileInput?.click()"><FileUp />{{ t('adv.tools.fromFile') }}</Button>
      <Button variant="ghost" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="!parsed?.entries.length" data-testid="otp-import" @click="importAll()">{{ t('adv.tools.importButton') }}</Button>
    </template>
  </Dialog>
</template>
