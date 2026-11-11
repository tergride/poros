<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import OtpTab from '@/components/adv/OtpTab.vue'
import SpeedCard from '@/components/adv/SpeedCard.vue'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui'

// «Tools» of the advanced mode: the codes of OTP, and the speed test of the chosen server.
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const tab = computed({
  get: () => String(route.params.tab ?? 'otp'),
  set: (next: string) => void router.push(`/adv/tools/${next}`),
})
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <Tabs v-model="tab">
      <TabsList>
        <TabsTrigger value="otp">{{ t('adv.tools.otp') }}</TabsTrigger>
        <TabsTrigger value="misc">{{ t('adv.tools.misc') }}</TabsTrigger>
      </TabsList>
    </Tabs>
    <OtpTab v-if="tab === 'otp'" />
    <div v-else class="min-h-0 flex-1 overflow-auto px-3.5 py-3.5">
      <div class="grid grid-cols-2 gap-3"><SpeedCard /></div>
    </div>
  </div>
</template>
