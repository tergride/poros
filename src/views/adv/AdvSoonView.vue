<script setup lang="ts">
import { Hammer } from '@lucide/vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

// A section the advanced mode has yet to fill: it says so rather than show nothing.
const route = useRoute()
const { t } = useI18n()
</script>

<template>
  <section class="grid flex-1 place-items-center p-10 text-center" data-testid="section-soon">
    <div class="grid max-w-[420px] justify-items-center gap-3">
      <Hammer class="size-8 text-t3" />
      <h2 class="font-disp text-xl font-normal">{{ t(`adv.nav.${route.params.section}`) }}</h2>
      <p class="text-t2">{{ t('adv.soon') }}</p>
    </div>
  </section>
</template>
