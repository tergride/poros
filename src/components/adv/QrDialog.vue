<script setup lang="ts">
import { Check, Copy, QrCode, TriangleAlert } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog, Textarea } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { reasonOf } from '@/composables/useReason'
import { qrPath } from '@/lib/qr'
import { appControls } from '@/platform'
import { MethodQREncode, ReasonQRTooLong } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// A link as a QR code and as text: a profile's, for a client on a phone, or a code of OTP's, for an authenticator.
// The core draws the code; a link longer than a QR code holds is said so instead.
const open = defineModel<boolean>('open', { default: false })
const props = defineProps<{ title: string; link: string; chip?: string; secret?: boolean; note?: string }>()

const core = useCoreStore()
const attempt = useAttempt()
const { t } = useI18n()
const rows = ref<string[]>([])
const tooLong = ref(false)
const copied = ref(false)

watch(
  () => [open.value, props.link] as const,
  async ([now, link]) => {
    rows.value = []
    tooLong.value = false
    copied.value = false
    if (!now || !link) return
    try {
      rows.value = (await core.core().call(MethodQREncode, { text: link })).rows
    } catch (error) {
      if (reasonOf(error) === ReasonQRTooLong) tooLong.value = true
      else await attempt(() => Promise.reject(error))
    }
  },
  { immediate: true },
)

// Four modules of quiet zone around the code, as readers expect.
const box = computed(() => `-4 -4 ${rows.value.length + 8} ${rows.value.length + 8}`)
const path = computed(() => qrPath(rows.value))

async function copy(): Promise<void> {
  await attempt(async () => {
    await appControls.copy(props.link)
    copied.value = true
  })
}
</script>

<template>
  <Dialog v-model:open="open" :title="title" class="w-[660px]">
    <template #icon><QrCode /></template>
    <template v-if="chip" #actions><span class="rounded-md bg-s4 px-2 py-1 text-[11.5px] text-t2">{{ chip }}</span></template>
    <div class="flex items-start gap-[18px]" data-testid="qr-dialog">
      <svg
        v-if="rows.length"
        :viewBox="box"
        width="220"
        height="220"
        role="img"
        :aria-label="t('qr.picture')"
        class="flex-none rounded-[10px] bg-white"
        shape-rendering="crispEdges"
        data-testid="qr-picture"
      >
        <path :d="path" fill="#0b1020" />
      </svg>
      <div
        v-else-if="tooLong"
        class="grid size-[220px] flex-none place-items-center rounded-[10px] border border-dashed border-line2 p-5 text-center text-xs text-t3"
        data-testid="qr-too-long"
      >
        {{ t('qr.tooLong') }}
      </div>
      <div v-else class="size-[220px] flex-none rounded-[10px] bg-s1" />
      <div class="grid min-w-0 flex-1 gap-2.5">
        <label class="grid gap-1.5">
          <span class="text-xs text-t3">{{ t('qr.link') }}</span>
          <Textarea :model-value="link" readonly mono class="h-[132px] resize-none break-all" spellcheck="false" data-testid="qr-link" />
        </label>
        <p v-if="secret" class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s1 px-3.5 py-2.5 text-[13px]">
          <TriangleAlert class="size-4 flex-none text-warn" />{{ t('qr.secret') }}
        </p>
        <p v-else-if="note" class="text-xs text-t3">{{ note }}</p>
      </div>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="open = false">{{ t('common.close') }}</Button>
      <Button variant="primary" data-testid="qr-copy" @click="copy()"><Check v-if="copied" /><Copy v-else />{{ t('qr.copy') }}</Button>
    </template>
  </Dialog>
</template>
