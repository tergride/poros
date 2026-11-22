<script setup lang="ts">
import { CircleAlert, ImageUp, Import, LoaderCircle, X } from '@lucide/vue'
import { useEventListener } from '@vueuse/core'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import SameNamePresetDialog from '@/components/presets/SameNamePresetDialog.vue'
import { Button, Dialog, Input } from '@/components/ui'
import { importAccept, importableIn, usePresetImport } from '@/composables/usePresetImport'
import type { Preset } from '@/platform/core-types'

// «Import a preset»: a picture of its QR code or its file, dropped on the field, picked by a click on it or pasted,
// or its link under it. The dialog closes on the preset added; what fails is said in it. A name of the user's preset
// puts the question of SameNamePresetDialog in its place, and Cancel there comes back to it.
const open = defineModel<boolean>('open', { default: false })
const emit = defineEmits<{ added: [preset: Preset, replaced: boolean] }>()
const { t } = useI18n()
const { busy, error, taken, fromText, fromFile } = usePresetImport()
const link = ref('')
const dragging = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const content = ref<HTMLElement | null>(null)

watch(open, (now) => {
  if (!now) return
  link.value = ''
  error.value = ''
  taken.value = null
  dragging.value = false
})

async function done(adding: Promise<Preset | null>): Promise<void> {
  const preset = await adding
  if (preset) finish(preset, false)
}

function finish(preset: Preset, replaced: boolean): void {
  open.value = false
  taken.value = null
  emit('added', preset, replaced)
}

function addLink(): void {
  if (link.value.trim()) void done(fromText(link.value))
}

// A file of another kind is still tried, so that the dialog says what it takes.
function take(files: FileList | null | undefined): void {
  const file = importableIn(files) ?? files?.[0]
  if (file) void done(fromFile(file))
}

function picked(event: Event): void {
  const input = event.target as HTMLInputElement
  take(input.files)
  input.value = ''
}

const carriesFiles = (event: DragEvent) => event.dataTransfer?.types.includes('Files') ?? false

function over(event: DragEvent): void {
  if (!carriesFiles(event)) return
  event.preventDefault()
  event.stopPropagation()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
  dragging.value = true
}

function leave(event: DragEvent): void {
  const zone = event.currentTarget as Node | null
  if (!zone?.contains(event.relatedTarget as Node | null)) dragging.value = false
}

function drop(event: DragEvent): void {
  if (!carriesFiles(event)) return
  event.preventDefault()
  event.stopPropagation()
  dragging.value = false
  take(event.dataTransfer?.files)
}

// A picture or a file pasted while the dialog is on top, wherever the focus is, is imported at once; pasted text goes
// into the field of the link, and outside it is added.
useEventListener(document, 'paste', (event: ClipboardEvent) => {
  if (open.value && !taken.value && !busy.value && !covered()) pasted(event)
})

// A modal dialog hides those under it from assistive technology, this one too when a link brings its own over it.
function covered(): boolean {
  return !!content.value?.closest('[aria-hidden="true"]')
}

function pasted(event: ClipboardEvent): void {
  const file = importableIn(event.clipboardData?.files)
  if (file) {
    event.preventDefault()
    void done(fromFile(file))
    return
  }
  const text = event.clipboardData?.getData('text/plain').trim() ?? ''
  if (!text || event.target instanceof HTMLInputElement) return
  event.preventDefault()
  link.value = text
  addLink()
}
</script>

<template>
  <Dialog :open="open && !taken" :title="t('settings.presets.importTitle')" class="w-[560px]" @update:open="(shown: boolean) => (open = shown)">
    <template #icon><Import /></template>
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="open = false"><X /></Button>
    </template>
    <div ref="content" class="grid gap-4" data-testid="import-preset">
      <button
        type="button"
        :disabled="busy"
        :class="[
          'grid h-[176px] w-full cursor-pointer content-center justify-items-center gap-1.5 rounded-xl border-2 border-dashed px-6 text-center transition-colors [&>*]:pointer-events-none',
          dragging ? 'border-act bg-act/10' : 'border-line2 bg-s1 hover:border-act/60 hover:bg-s3',
        ]"
        data-testid="import-drop"
        @click="fileInput?.click()"
        @dragover="over"
        @dragleave="leave"
        @drop="drop"
      >
        <span class="mb-1.5 grid size-11 place-items-center rounded-full bg-s4 text-act">
          <LoaderCircle v-if="busy" class="size-5 animate-spin" />
          <ImageUp v-else class="size-5" />
        </span>
        <span class="text-[14.5px] font-medium text-t1">{{ busy ? t('settings.presets.reading') : t('settings.presets.dropTitle') }}</span>
        <span class="text-xs text-t3">{{ t('settings.presets.dropHint') }}</span>
        <span class="mt-1 text-xs text-t3">
          {{ t('settings.presets.pasteHint') }}
          <kbd class="ml-1 rounded-md border border-line2 bg-s3 px-1.5 py-px font-mono text-[11px] text-t2">Ctrl+V</kbd>
        </span>
      </button>
      <input ref="fileInput" type="file" class="hidden" :accept="importAccept" data-testid="import-file" @change="picked" />

      <div class="flex items-center gap-3 text-xs text-t3">
        <span class="h-px flex-1 bg-line" />{{ t('settings.presets.orLink') }}<span class="h-px flex-1 bg-line" />
      </div>
      <div class="flex gap-2">
        <Input
          v-model="link"
          mono
          class="min-w-0 flex-1"
          placeholder="poros://route/…"
          :aria-label="t('settings.presets.linkLabel')"
          spellcheck="false"
          data-testid="import-link"
          @keydown.enter.prevent="addLink"
        />
        <Button variant="primary" :disabled="busy || !link.trim()" data-testid="import-add" @click="addLink">{{ t('common.add') }}</Button>
      </div>

      <p v-if="error" class="flex items-start gap-2 text-[13px] text-bad" role="alert" data-testid="import-error">
        <CircleAlert class="mt-0.5 size-4 flex-none" />{{ error }}
      </p>
    </div>
  </Dialog>
  <SameNamePresetDialog
    :incoming="open ? taken : null"
    @cancel="taken = null"
    @refreshed="(read) => (taken = read.same ? read : null)"
    @done="finish"
  />
</template>
