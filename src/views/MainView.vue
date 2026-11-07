<script setup lang="ts">
import { onMounted, ref } from 'vue'

import AdminDialog from '@/components/main/AdminDialog.vue'
import IpDialog from '@/components/main/IpDialog.vue'
import ModesDialog from '@/components/main/ModesDialog.vue'
import Portal from '@/components/main/Portal.vue'
import ServerList from '@/components/main/ServerList.vue'
import { MethodNetworkCheck } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

const core = useCoreStore()
const ipOpen = ref(false)

// The core checks after connecting and disconnecting; the first look at the address is the window's to ask for.
onMounted(() => {
  if (core.settings?.checkIp && core.network.checkedAt === 0 && !core.network.checking) {
    void core.core().call(MethodNetworkCheck)
  }
})
</script>

<template>
  <div class="flex min-h-0 flex-1">
    <Portal @ip="ipOpen = true" />
    <ServerList />
    <ModesDialog />
    <AdminDialog />
    <IpDialog v-model:open="ipOpen" />
  </div>
</template>
