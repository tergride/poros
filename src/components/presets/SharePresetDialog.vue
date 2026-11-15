<script setup lang="ts">
import { Check, Copy, FileDown, ImageIcon, Share2, X } from '@lucide/vue'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog, Input } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { reasonOf } from '@/composables/useReason'
import { qrPath, qrPng } from '@/lib/qr'
import { appControls } from '@/platform'
import type { Preset } from '@/platform/core-types'
import { MethodPresetsExport, MethodPresetsFile, MethodQREncode, ReasonQRTooLong } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// «Share» of a user's preset: its QR code with the button that copies the picture of it for a message, its
// poros://route/ link to copy and its file of JSON. Endpoints are this computer's profiles: they stay out, and the dialog
// says so.
const open = defineModel<boolean>('open', { default: false })
const props = defineProps<{ preset: Preset | null }>()

const core = useCoreStore()
const attempt = useAttempt()
const { t } = useI18n()
const link = ref('')
const rows = ref<string[]>([])
const tooLong = ref(false)
const copied = ref<'' | 'link' | 'qr'>('')
const saved = ref('')

watch(
  () => [open.value, props.preset?.id] as const,
  async ([now, id]) => {
    link.value = ''
    rows.value = []
    tooLong.value = false
    copied.value = ''
    saved.value = ''
    if (!now || !id) return
    await attempt(async () => {
      link.value = (await core.core().call(MethodPresetsExport, { id })).link
      try {
        rows.value = (await core.core().call(MethodQREncode, { text: link.value })).rows
      } catch (error) {
        if (reasonOf(error) !== ReasonQRTooLong) throw error
        tooLong.value = true
      }
    })
  },
  { immediate: true },
)

// Four modules of quiet zone around the code, as readers expect.
const box = computed(() => `-4 -4 ${rows.value.length + 8} ${rows.value.length + 8}`)
const path = computed(() => qrPath(rows.value))

let timer: ReturnType<typeof setTimeout> | undefined
function flash(what: 'link' | 'qr'): void {
  copied.value = what
  clearTimeout(timer)
  timer = setTimeout(() => (copied.value = ''), 2000)
}
onBeforeUnmount(() => clearTimeout(timer))

const copyLink = () =>
  attempt(async () => {
    await appControls.copy(link.value)
    flash('link')
  })

const copyQr = () =>
  attempt(async () => {
    await navigator.clipboard.write([new ClipboardItem({ 'image/png': qrPng(rows.value) })])
    flash('qr')
  })

const exportJson = () =>
  attempt(async () => {
    if (!props.preset) return
    const file = await core.core().call(MethodPresetsFile, { id: props.preset.id })
    const where = await appControls.saveFile(file.name, t('settings.presets.fileKind'), file.text)
    if (where) saved.value = where
  })
</script>

<template>
  <Dialog v-model:open="open" :title="t('settings.presets.shareTitle', { name: preset?.name ?? '' })" class="w-[560px]">
    <template #icon><Share2 /></template>
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="open = false"><X /></Button>
    </template>
    <div class="grid gap-5" data-testid="share-preset">

      <section class="flex gap-5 rounded-xl border border-line bg-s1 p-4">
        <svg
          v-if="rows.length"
          :viewBox="box"
          width="168"
          height="168"
          role="img"
          :aria-label="t('qr.picture')"
          class="flex-none rounded-[10px] bg-white"
          shape-rendering="crispEdges"
          data-testid="share-qr"
        >
          <path :d="path" fill="#0b1020" />
        </svg>
        <div
          v-else-if="tooLong"
          class="grid size-[168px] flex-none place-items-center rounded-[10px] border border-dashed border-line2 p-4 text-center text-xs text-t3"
        >
          {{ t('qr.tooLong') }}
        </div>
        <div v-else class="size-[168px] flex-none rounded-[10px] bg-s3" />
        <div class="flex min-w-0 flex-1 flex-col py-0.5">
          <h3 class="text-[14.5px] font-medium text-t1">{{ t('qr.picture') }}</h3>
          <p class="mt-1.5 text-[13px] leading-[1.45] text-t3">{{ t('settings.presets.shareQrHint') }}</p>
          <Button variant="primary" class="mt-auto self-start" :disabled="!rows.length" data-testid="share-copy-qr" @click="copyQr">
            <Check v-if="copied === 'qr'" /><ImageIcon v-else />{{ copied === 'qr' ? t('settings.presets.copiedQr') : t('settings.presets.copyQr') }}
          </Button>
        </div>
      </section>

      <label class="grid gap-2">
        <span class="text-[14.5px] font-medium text-t1">{{ t('qr.link') }}</span>
        <span class="flex gap-2">
          <Input :model-value="link" readonly mono class="min-w-0 flex-1" spellcheck="false" data-testid="share-link" @focus="($event.target as HTMLInputElement).select()" />
          <Button :disabled="!link" data-testid="share-copy-link" @click="copyLink">
            <Check v-if="copied === 'link'" class="text-ok" /><Copy v-else />{{ copied === 'link' ? t('settings.presets.copied') : t('common.copy') }}
          </Button>
        </span>
      </label>
    </div>
    <template #footer>
      <p v-if="saved" class="sp flex min-w-0 items-center gap-1.5 text-xs text-ok" data-testid="share-saved">
        <Check class="size-3.5 flex-none" /><span class="truncate" :title="saved">{{ t('settings.presets.savedFile', { path: saved }) }}</span>
      </p>
      <p v-else class="sp min-w-0 text-xs text-t3">{{ t('settings.presets.shareFileHint') }}</p>
      <Button class="flex-none" :disabled="!link" data-testid="share-export" @click="exportJson"><FileDown />{{ t('settings.presets.exportJson') }}</Button>
    </template>
  </Dialog>
</template>
