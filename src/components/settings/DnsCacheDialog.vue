<script setup lang="ts">
import { X } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import OptionRow from '@/components/settings/OptionRow.vue'
import { Button, Dialog, Input, Switch } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'

// The cache of DNS answers and how long a query waits, for the draft of the settings page. What works only with the
// cache is inactive without it; an optimistic cache needs answers that get outdated, so not with ignored lifetimes.
const open = defineModel<boolean>('open', { default: false })
const { t } = useI18n()
const settings = {
  cache: useSetting('dnsCache'),
  size: useSetting('dnsCacheSize'),
  ignoreTtl: useSetting('dnsIgnoreTtl'),
  optimistic: useSetting('dnsOptimistic'),
  timeout: useSetting('dnsTimeout'),
  subnet: useSetting('dnsClientSubnet'),
}

const cache = ref(true)
const size = ref('')
const ignoreTtl = ref(false)
const optimistic = ref(false)
const timeout = ref('')
const subnet = ref('')
watch(open, (shown) => {
  if (!shown) return
  cache.value = settings.cache.value
  size.value = String(settings.size.value)
  ignoreTtl.value = settings.ignoreTtl.value
  optimistic.value = settings.optimistic.value
  timeout.value = String(settings.timeout.value)
  subnet.value = settings.subnet.value
})
watch(ignoreTtl, (on) => {
  if (on) optimistic.value = false
})

const whole = (text: string, max: number) => (/^\d+$/.test(text.trim()) && Number(text) >= 1 && Number(text) <= max ? Number(text) : null)
const problem = computed(() => {
  if (whole(size.value, 1 << 20) === null) return t('dnsCache.wrongSize')
  if (whole(timeout.value, 60) === null) return t('dnsCache.wrongTimeout')
  return ''
})

function done(): void {
  if (problem.value) return
  settings.cache.value = cache.value
  settings.size.value = whole(size.value, 1 << 20)!
  settings.ignoreTtl.value = ignoreTtl.value
  settings.optimistic.value = optimistic.value
  settings.timeout.value = whole(timeout.value, 60)!
  settings.subnet.value = subnet.value.trim()
  open.value = false
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('dnsCache.title')" class="w-[640px]">
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="open = false"><X /></Button>
    </template>
    <OptionRow :title="t('dnsCache.cache')" :hint="t('dnsCache.cacheHint')"><Switch v-model="cache" data-testid="dns-cache-on" /></OptionRow>
    <OptionRow :title="t('dnsCache.size')" :hint="t('dnsCache.sizeHint')">
      <Input v-model="size" mono class="w-[130px]" :disabled="!cache" :aria-label="t('dnsCache.size')" data-testid="dns-cache-size" />
    </OptionRow>
    <OptionRow :title="t('dnsCache.ignoreTtl')" :hint="t('dnsCache.ignoreTtlHint')"><Switch v-model="ignoreTtl" :disabled="!cache" /></OptionRow>
    <OptionRow :title="t('dnsCache.optimistic')" :hint="t(ignoreTtl ? 'dnsCache.optimisticOff' : 'dnsCache.optimisticHint')">
      <Switch v-model="optimistic" :disabled="!cache || ignoreTtl" data-testid="dns-optimistic" />
    </OptionRow>
    <OptionRow :title="t('dnsCache.timeout')" :hint="t('dnsCache.timeoutHint')">
      <Input v-model="timeout" mono class="w-[130px]" :aria-label="t('dnsCache.timeout')" data-testid="dns-timeout" />
    </OptionRow>
    <OptionRow :title="t('dnsCache.subnet')" :hint="t('dnsCache.subnetHint')">
      <Input v-model="subnet" mono class="w-[130px]" placeholder="203.0.113.0/24" :aria-label="t('dnsCache.subnet')" />
    </OptionRow>
    <p v-if="problem" class="mt-2 text-xs text-warn" data-testid="dns-cache-problem">{{ problem }}</p>
    <template #footer>
      <Button variant="ghost" class="sp" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="!!problem" data-testid="dns-cache-done" @click="done">{{ t('common.done') }}</Button>
    </template>
  </Dialog>
</template>
