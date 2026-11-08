<script setup lang="ts">
import ServerList from '@/components/main/ServerList.vue'
</script>

<template>
  <!-- The servers by group, as in the simple mode, until the table of profiles takes their place. -->
  <ServerList :banners="false" />
</template>
