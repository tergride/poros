<script setup lang="ts">
import { CircleAlert, FileUp, Import, LoaderCircle, X } from '@lucide/vue'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog } from '@/components/ui'
import { useFailure } from '@/composables/useReason'
import type { Endpoint } from '@/platform/core-types'
import { MethodEndpointsImport } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// «Import»: the file of an endpoint or an .ovpn, dropped on the field or picked by a click on it. The dialog closes on
// the endpoint added; one without the data for signing in asks for it next.
const open = defineModel<boolean>('open', { default: false })
const emit = defineEmits<{ added: [endpoint: Endpoint] }>()
const core = useCoreStore()
const failureOf = useFailure()
const { t } = useI18n()
const busy = ref(false)
const error = ref('')
const dragging = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)

watch(open, (now) => {
  if (!now) return
  error.value = ''
  dragging.value = false
})

async function take(file: File | undefined): Promise<void> {
  if (!file || busy.value) return
  busy.value = true
  error.value = ''
  try {
    const endpoint = await core.core().call(MethodEndpointsImport, { text: await file.text() })
    open.value = false
    emit('added', endpoint)
  } catch (failure) {
    error.value = failureOf(failure)
  } finally {
    busy.value = false
  }
}

function picked(event: Event): void {
  const input = event.target as HTMLInputElement
  void take(input.files?.[0])
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
  void take(event.dataTransfer?.files[0])
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('endpoints.importTitle')" class="w-[560px]">
    <template #icon><Import /></template>
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="open = false"><X /></Button>
    </template>
    <div class="grid gap-3" data-testid="endpoint-import">
      <button
        type="button"
        :disabled="busy"
        :class="[
          'grid h-[160px] w-full cursor-pointer content-center justify-items-center gap-1.5 rounded-xl border-2 border-dashed px-6 text-center transition-colors [&>*]:pointer-events-none',
          dragging ? 'border-act bg-act/10' : 'border-line2 bg-s1 hover:border-act/60 hover:bg-s3',
        ]"
        data-testid="endpoint-import-drop"
        @click="fileInput?.click()"
        @dragover="over"
        @dragleave="leave"
        @drop="drop"
      >
        <span class="mb-1.5 grid size-11 place-items-center rounded-full bg-s4 text-act">
          <LoaderCircle v-if="busy" class="size-5 animate-spin" />
          <FileUp v-else class="size-5" />
        </span>
        <span class="text-[14.5px] font-medium text-t1">{{ busy ? t('endpoints.reading') : t('endpoints.dropTitle') }}</span>
        <span class="text-xs text-t3">{{ t('endpoints.dropHint') }}</span>
      </button>
      <input ref="fileInput" type="file" class="hidden" accept=".json,.ovpn,.conf,application/json" data-testid="endpoint-import-file" @change="picked" />
      <p class="text-xs text-t3">{{ t('endpoints.importHint') }}</p>
      <p v-if="error" class="flex items-start gap-2 text-[13px] text-bad" role="alert" data-testid="endpoint-import-error">
        <CircleAlert class="mt-0.5 size-4 flex-none" />{{ error }}
      </p>
    </div>
  </Dialog>
</template>
