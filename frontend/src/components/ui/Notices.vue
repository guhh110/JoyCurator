<script setup lang="ts">
defineProps<{ error?: string; success?: string }>()
</script>
<template>
  <p v-if="error" class="notice error" role="alert">{{ error }}</p>
  <p v-if="success" class="notice good" role="status">{{ success }}</p>
</template>
