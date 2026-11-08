<script setup lang="ts">
import { FolderPlus, Link } from '@lucide/vue'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog, Input } from '@/components/ui'
import { useProfileActions } from '@/composables/useProfileActions'
import { useUiStore } from '@/stores/ui'

// «Subscription or group»: a subscription goes through the dialog that tries its link, a group of the user's needs
// only a name.
const open = defineModel<boolean>('open', { default: false })
const ui = useUiStore()
const actions = useProfileActions()
const { t } = useI18n()
const name = ref('')
watch(open, () => (name.value = ''))

function subscription(): void {
  open.value = false
  ui.addSubscription = true
}

async function create(): Promise<void> {
  if (!name.value.trim()) return
  open.value = false
  await actions.newGroup(name.value.trim())
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('profiles.newTitle')">
    <template #icon><FolderPlus /></template>
    <div class="grid gap-4">
      <div class="grid gap-2">
        <p class="text-t2">{{ t('profiles.newSubscriptionText') }}</p>
        <Button class="justify-self-start" data-testid="new-subscription" @click="subscription()"><Link />{{ t('profiles.newSubscription') }}</Button>
      </div>
      <div class="h-px bg-line" />
      <label class="grid gap-1.5">
        <span class="text-t2">{{ t('profiles.newGroupText') }}</span>
        <Input v-model="name" :placeholder="t('list.groupNamePlaceholder')" data-testid="new-group-name" @keydown.enter="create()" />
      </label>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="!name.trim()" data-testid="new-group-create" @click="create()"><FolderPlus />{{ t('profiles.newGroup') }}</Button>
    </template>
  </Dialog>
</template>
