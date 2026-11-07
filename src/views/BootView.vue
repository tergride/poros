<script setup lang="ts">
import { DatabaseBackup, LogOut, RefreshCw } from '@lucide/vue'
import { computed, ref, watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import mark from '@/assets/img/mark.png'
import { Button } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useFailure } from '@/composables/useReason'
import { StatusDataDamaged, appControls } from '@/platform'
import { useCoreStore } from '@/stores/core'

const core = useCoreStore()
const router = useRouter()
const attempt = useAttempt()
const failure = useFailure()
const { t } = useI18n()
const busy = ref(false)
// The boot screen has no notice line: a failed restart is told here.
const failed = ref('')

watchEffect(() => {
  if (core.loaded && core.settings) void router.replace(core.settings.onboardingDone ? '/main' : '/wizard/1')
})

const damaged = computed(() => core.status.reason === StatusDataDamaged)
const quit = () => attempt(() => appControls.exit())

async function run(action: () => Promise<unknown>): Promise<void> {
  busy.value = true
  failed.value = ''
  try {
    await action()
  } catch (error) {
    failed.value = failure(error)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="grid flex-1 place-items-center p-8">
    <div class="flex max-w-xl flex-col items-center gap-4 text-center">
      <img :src="mark" alt="" class="h-16 w-auto opacity-90" />
      <p v-if="core.status.state !== 'down'" class="text-t2" data-testid="boot-state">{{ t('boot.starting') }}</p>
      <template v-else>
        <p class="text-t2" data-testid="boot-state">{{ damaged ? t('boot.damaged') : t('boot.down') }}</p>
        <div class="flex flex-wrap justify-center gap-2">
          <Button v-if="damaged" variant="primary" :disabled="busy" data-testid="reset-data" @click="run(() => core.core().resetData())">
            <DatabaseBackup />{{ t('boot.resetData') }}
          </Button>
          <Button v-else :disabled="busy" @click="run(() => core.core().restart())"><RefreshCw />{{ t('boot.restart') }}</Button>
          <Button variant="ghost" @click="quit()"><LogOut />{{ t('boot.exit') }}</Button>
        </div>
        <p v-if="failed" class="text-sm text-warn">{{ failed }}</p>
        <details v-if="core.status.error" class="w-full text-left text-xs text-t3">
          <summary class="cursor-pointer text-center">{{ t('boot.details') }}</summary>
          <p class="mt-2 font-mono break-words whitespace-pre-wrap text-bad">{{ core.status.error }}</p>
        </details>
      </template>
    </div>
  </section>
</template>
