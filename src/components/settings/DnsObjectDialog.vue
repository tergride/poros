<script setup lang="ts">
import { Trash2, TriangleAlert, X } from '@lucide/vue'
import { useDebounceFn } from '@vueuse/core'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog, Textarea } from '@/components/ui'
import { useFailure } from '@/composables/useReason'
import { useSetting } from '@/composables/useSetting'
import { jsonErrorLine } from '@/lib/profileForm'
import { MethodDNSCheck } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// The dns object of sing-box as a whole, for what the page has not. A JSON mistake is named by its line; the core
// reads the object in a connection as it is typed, and Done waits for it to agree.
const open = defineModel<boolean>('open', { default: false })
const core = useCoreStore()
const failure = useFailure()
const { t } = useI18n()
const object = useSetting('dnsObject')

const example = `{
  "servers": [
    { "type": "https", "tag": "remote", "server": "1.1.1.1", "detour": "proxy" },
    { "type": "local", "tag": "local" }
  ],
  "rules": [
    { "domain_suffix": ["lan"], "server": "local" }
  ],
  "final": "remote"
}`
const text = ref('')
const problem = ref('')
const checking = ref(false)
watch(open, (shown) => {
  if (!shown) return
  text.value = object.value || example
  problem.value = ''
})

async function check(): Promise<boolean> {
  try {
    JSON.parse(text.value)
  } catch (error) {
    const line = jsonErrorLine(text.value, error)
    problem.value = line ? t('profileEditor.jsonLine', { line }) : t('profileEditor.notJson')
    return false
  }
  checking.value = true
  try {
    await core.core().call(MethodDNSCheck, { object: text.value })
    problem.value = ''
    return true
  } catch (error) {
    problem.value = failure(error)
    return false
  } finally {
    checking.value = false
  }
}
const checkSoon = useDebounceFn(() => (open.value ? check() : null), 400)
watch(text, () => void checkSoon())

async function done(): Promise<void> {
  if (!(await check())) return
  object.value = text.value
  open.value = false
}

function remove(): void {
  object.value = ''
  open.value = false
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('dnsObject.title')" class="w-[720px]">
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="open = false"><X /></Button>
    </template>
    <div class="grid gap-2.5">
      <p class="text-xs text-t3">{{ t('dnsObject.hint') }}</p>
      <Textarea v-model="text" mono class="h-[280px] resize-none" spellcheck="false" :aria-label="t('dnsObject.label')" data-testid="dns-object-text" />
      <div v-if="problem" class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-2.5" role="alert" data-testid="dns-object-problem">
        <TriangleAlert class="size-4 flex-none text-warn" /><span>{{ problem }}</span>
      </div>
    </div>
    <template #footer>
      <Button v-if="object" variant="ghost" class="sp text-bad" data-testid="dns-object-remove" @click="remove"><Trash2 />{{ t('dnsObject.remove') }}</Button>
      <Button variant="ghost" :class="!object && 'sp'" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="checking || !!problem" data-testid="dns-object-done" @click="done">{{ t('common.done') }}</Button>
    </template>
  </Dialog>
</template>
