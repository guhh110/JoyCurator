<script setup lang="ts">
defineProps<{ title: string; description?: string; eyebrow?: string }>()
</script>
<template>
  <header class="shared-page-heading">
    <div>
      <span v-if="eyebrow" class="eyebrow">{{ eyebrow }}</span>
      <h1>{{ title }}<span class="heading-dot">.</span></h1>
      <p v-if="description" class="muted">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="shared-page-actions">
      <slot name="actions" />
    </div>
  </header>
</template>
<style scoped>
.shared-page-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 20px;
}
.shared-page-heading h1 {
  margin: 8px 0 0;
}
.shared-page-heading p {
  margin: 10px 0 0;
  line-height: 1.7;
}
.shared-page-actions {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
}
@media (max-width: 600px) {
  .shared-page-heading {
    align-items: flex-start;
  }
  .shared-page-actions {
    width: 100%;
  }
}
</style>
