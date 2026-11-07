<script setup lang="ts">
import { Play, ShieldAlert } from '@lucide/vue'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Checkbox, Dialog } from '@/components/ui'
import { useConnect } from '@/composables/useConnect'
import { useCoreStore } from '@/stores/core'

// The first Start with both switches off: what should go through the server.
const core = useCoreStore()
const connect = useConnect()
const { t } = useI18n()
const tun = ref(true)
const system = ref(false)

watch(
  () => connect.modesOpen,
  (shown) => {
    if (shown) {
      tun.value = true
      system.value = false
    }
  },
)
</script>

<template>
  <Dialog v-model:open="connect.modesOpen" :title="t('modes.title')">
    <div class="grid gap-2.5">
      <label class="flex cursor-pointer gap-3.5 rounded-xl border border-line bg-s2 px-4 py-[15px] hover:border-line2">
        <Checkbox v-model="tun" />
        <span>
          <strong class="mb-0.5 flex items-center gap-1.5 font-medium">
            TUN<ShieldAlert v-if="!core.status.elevated" class="size-3.5 text-warn" />
          </strong>
          <span class="text-xs text-t3">{{ t('modes.tun') }}</span>
        </span>
      </label>
      <label class="flex cursor-pointer gap-3.5 rounded-xl border border-line bg-s2 px-4 py-[15px] hover:border-line2">
        <Checkbox v-model="system" />
        <span>
          <strong class="mb-0.5 block font-medium">System</strong>
          <span class="text-xs text-t3">{{ t('modes.system') }}</span>
        </span>
      </label>
      <p class="text-xs text-t3">{{ t('modes.note') }}</p>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="connect.modesOpen = false">{{ t('common.cancel') }}</Button>
      <Button variant="ghost" data-testid="modes-none" @click="connect.chooseModes(false, false)">{{ t('modes.none') }}</Button>
      <Button variant="primary" data-testid="modes-connect" @click="connect.chooseModes(tun, system)">
        <Play class="size-4 fill-current" />{{ t('modes.connect') }}
      </Button>
    </template>
  </Dialog>
</template>
