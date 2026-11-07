<script setup lang="ts">
import { RefreshCw } from '@lucide/vue'
import { watchEffect } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import mark from '@/assets/img/mark.png'
import { Button } from '@/components/ui'
import { useCoreStore } from '@/stores/core'

const core = useCoreStore()
const router = useRouter()
const { t } = useI18n()

watchEffect(() => {
  if (core.loaded && core.settings) void router.replace(core.settings.onboardingDone ? '/main' : '/wizard/1')
})
</script>

<template>
  <section class="grid flex-1 place-items-center">
    <div class="flex max-w-xl flex-col items-center gap-4 text-center">
      <img :src="mark" alt="" class="h-16 w-auto opacity-90" />
      <p v-if="core.status.state !== 'down'" class="text-t2" data-testid="boot-state">{{ t('boot.starting') }}</p>
      <template v-else>
        <p class="text-t2">{{ t('boot.down') }}</p>
        <p v-if="core.status.error" class="font-mono text-xs break-all text-bad">{{ core.status.error }}</p>
        <Button @click="core.client?.restart()"><RefreshCw />{{ t('boot.restart') }}</Button>
      </template>
    </div>
  </section>
</template>
