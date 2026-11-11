<script setup lang="ts">
import { Hammer } from '@lucide/vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import OtpTab from '@/components/adv/OtpTab.vue'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui'

// «Tools» of the advanced mode: the codes of OTP, and the rest.
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
    <section v-else class="grid flex-1 place-items-center p-10 text-center" data-testid="section-soon">
      <div class="grid max-w-[420px] justify-items-center gap-3">
        <Hammer class="size-8 text-t3" />
        <p class="text-t2">{{ t('adv.soon') }}</p>
      </div>
    </section>
  </div>
</template>
