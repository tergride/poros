<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import SubscriptionLink from '@/components/main/SubscriptionLink.vue'
import { Button, Dialog, Input } from '@/components/ui'

const open = defineModel<boolean>('open', { default: false })
const { t } = useI18n()
const name = ref('')
const link = ref<InstanceType<typeof SubscriptionLink> | null>(null)
// A new SubscriptionLink each time the dialog opens, so nothing of the last link stays.
const generation = ref(0)

watch(open, (shown) => {
  if (shown) {
    name.value = ''
    generation.value++
  }
})

async function add(): Promise<void> {
  await link.value?.add()
  if (link.value?.added) open.value = false
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('add.title')">
    <div class="grid gap-3">
      <label class="flex flex-col gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('add.link') }}</span>
        <SubscriptionLink :key="generation" ref="link" manual :name="name" />
      </label>
      <label class="flex flex-col gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('add.name') }}</span>
        <Input v-model="name" :placeholder="t('add.namePlaceholder')" />
      </label>
      <p class="text-xs text-t3">{{ t('add.note') }}</p>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="link?.busy" data-testid="add-subscription" @click="add">{{ t('common.add') }}</Button>
    </template>
  </Dialog>
</template>
