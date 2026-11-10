<script setup lang="ts">
import { Check, Copy, Plug, X } from '@lucide/vue'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { localProxyHost } from '@/platform/core'
import { useCoreStore } from '@/stores/core'

// What the local proxy of the portal's hint is and how to use it: the address for a program's settings and the links
// to paste, each copied by a click. HTTP and SOCKS5 share the port.
const open = defineModel<boolean>('open', { default: false })
const core = useCoreStore()
const attempt = useAttempt()
const { t } = useI18n()

const address = computed(() => `${localProxyHost}:${core.proxyPort}`)
// With a password on the port the links carry it, so that a program takes them whole.
const links = computed(() => {
  const s = core.settings
  const user = s?.proxyAuth ? `${encodeURIComponent(s.proxyUsername)}:${encodeURIComponent(s.proxyPassword)}@` : ''
  return [
    { kind: 'HTTP', link: `http://${user}${address.value}` },
    { kind: 'SOCKS5', link: `socks5://${user}${address.value}` },
  ]
})
const lan = computed(() => core.settings?.allowLan ?? false)

const copied = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
function copy(link: string): Promise<void> {
  return attempt(async () => {
    await navigator.clipboard.writeText(link)
    copied.value = link
    clearTimeout(timer)
    timer = setTimeout(() => (copied.value = ''), 2000)
  })
}
watch(open, () => (copied.value = ''))
onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <Dialog v-model:open="open" :title="t('proxy.title')" class="w-[540px]">
    <template #icon><Plug /></template>
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="open = false"><X /></Button>
    </template>
    <div class="grid gap-3" data-testid="proxy-dialog">
      <p class="text-t2">{{ t('proxy.lead', { address }) }}</p>
      <p class="text-t2">{{ t('proxy.how', { host: localProxyHost, port: core.proxyPort }) }}</p>
      <div class="overflow-hidden rounded-[10px] border border-line">
        <button
          v-for="item in links"
          :key="item.kind"
          type="button"
          class="grid w-full cursor-pointer grid-cols-[64px_minmax(0,1fr)_auto] items-center gap-3 border-t border-line bg-s3 px-3.5 py-2.5 text-left first:border-t-0 hover:bg-s4"
          :aria-label="t('proxy.copy', { kind: item.kind })"
          :data-testid="`proxy-${item.kind.toLowerCase()}`"
          @click="copy(item.link)"
        >
          <span class="text-xs font-medium text-t3">{{ item.kind }}</span>
          <span class="truncate font-mono text-[13px]">{{ item.link }}</span>
          <span v-if="copied === item.link" class="flex items-center gap-1.5 text-xs text-ok"><Check class="size-4" />{{ t('proxy.copied') }}</span>
          <Copy v-else class="size-4 text-t3" />
        </button>
      </div>
      <p class="text-xs text-t3">{{ t('proxy.modes') }}</p>
      <p class="text-xs text-t3" data-testid="proxy-lan">{{ lan ? t('proxy.lan', { port: core.proxyPort }) : t('proxy.lanOff') }}</p>
    </div>
  </Dialog>
</template>
