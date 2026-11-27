<script setup lang="ts">
import { Eraser, LoaderCircle, LogOut, ShieldAlert } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog, vTooltip } from '@/components/ui'
import { useCleanup } from '@/composables/useCleanup'
import { useFailure } from '@/composables/useReason'
import { appControls } from '@/platform'
import type { CleanupItem } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// What Poros left in Windows outside its folder, before anything goes, then what a run could not remove. The folder
// stays: the dialog only says so.
const open = defineModel<boolean>('open', { default: false })

const core = useCoreStore()
const { t } = useI18n()
const failure = useFailure()
const cleanup = useCleanup()
const error = ref('')

watch(open, (now) => {
  if (!now) return
  error.value = ''
  cleanup.scan().catch((e: unknown) => (error.value = failure(e)))
})

const items = computed(() => cleanup.left.value ?? cleanup.found.value ?? [])
// A core with rights already asks Windows for nothing.
const needsRights = computed(() => !cleanup.left.value && !core.status.elevated && items.value.some((item) => item.rights))

function why(item: CleanupItem): string {
  return item.error ? t('cleanup.leftError', { error: item.error }) : t('cleanup.leftRights')
}

async function run(): Promise<void> {
  error.value = ''
  try {
    await cleanup.run()
  } catch (e) {
    error.value = failure(e)
  }
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('cleanup.title')" class="w-[560px]">
    <template #icon><Eraser /></template>
    <div class="grid gap-3" data-testid="cleanup-dialog">
      <p class="text-t2">{{ t('cleanup.lead') }}</p>
      <p v-if="!cleanup.found.value && !error" class="flex items-center gap-2 text-t3">
        <LoaderCircle class="size-4 animate-spin" />{{ t('cleanup.scanning') }}
      </p>
      <p v-else-if="cleanup.found.value && items.length === 0" class="text-t3" data-testid="cleanup-nothing">{{ t('cleanup.nothing') }}</p>
      <template v-else-if="items.length">
        <p class="text-[12.5px] text-t2">{{ cleanup.left.value ? t('cleanup.left') : t('cleanup.found') }}</p>
        <ul class="grid gap-1.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3" data-testid="cleanup-items">
          <li v-for="item in items" :key="item.kind" class="flex items-start gap-3" :data-testid="`cleanup-${item.kind}`">
            <span class="grid flex-1 gap-0.5">
              <span>
                {{ t(`cleanup.kind.${item.kind}`) }}<span v-if="item.count > 1" class="ml-1.5 text-t3">{{ item.count }}</span>
              </span>
              <span v-if="cleanup.left.value" class="text-xs text-warn">{{ why(item) }}</span>
            </span>
            <ShieldAlert
              v-if="item.rights && !cleanup.left.value"
              v-tooltip="t('cleanup.rights')"
              class="mt-0.5 size-4 shrink-0 text-t3"
              :aria-label="t('cleanup.rights')"
            />
          </li>
        </ul>
        <p v-if="needsRights" class="text-xs text-t3">{{ t('cleanup.rightsNote') }}</p>
      </template>
      <p v-if="error" class="rounded-[10px] border border-line bg-s1 px-3.5 py-2.5 text-[13px] text-warn" data-testid="cleanup-error">{{ error }}</p>
    </div>
    <template #footer>
      <template v-if="cleanup.left.value">
        <Button variant="primary" class="ml-auto" data-testid="cleanup-exit" @click="appControls.exit()"><LogOut />{{ t('cleanup.exit') }}</Button>
      </template>
      <template v-else>
        <Button variant="ghost" class="sp" :disabled="cleanup.busy.value" @click="open = false">{{ t('common.cancel') }}</Button>
        <Button variant="primary" :disabled="cleanup.busy.value || !cleanup.found.value" data-testid="cleanup-run" @click="run()">
          <LoaderCircle v-if="cleanup.busy.value" class="animate-spin" /><Eraser v-else />{{ t('cleanup.run') }}
        </Button>
      </template>
    </template>
  </Dialog>
</template>
