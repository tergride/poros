<script setup lang="ts">
defineProps<{ title?: string }>()
</script>

<template>
  <section class="mb-3 rounded-xl border border-line bg-s2">
    <h3 v-if="title" class="px-4 pt-3 text-[13.5px] font-semibold">{{ title }}</h3>
    <div class="px-4 pt-1.5 pb-3"><slot /></div>
  </section>
</template>
