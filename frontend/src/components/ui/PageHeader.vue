<script setup lang="ts">
defineProps<{ title: string; description?: string; eyebrow?: string }>()
</script>
<template>
  <header
    class="shared-page-heading"
    :class="{ 'has-description': description }"
  >
    <div class="heading-title">
      <h1>{{ title }}<span class="heading-dot">.</span></h1>
      <span v-if="eyebrow" class="heading-eyebrow">{{ eyebrow }}</span>
    </div>
    <p v-if="description" class="heading-description muted">
      {{ description }}
    </p>
    <div v-if="$slots.actions" class="shared-page-actions">
      <slot name="actions" />
    </div>
  </header>
</template>
<style scoped>
.shared-page-heading {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 4px 16px;
  margin: 0 0 8px;
  min-width: 0;
  flex-shrink: 0;
}
.heading-title {
  display: flex;
  align-items: baseline;
  gap: 12px;
  min-width: 0;
}
h1 {
  margin: 0;
  font-size: 23px;
  line-height: 1.3;
  white-space: nowrap;
}
.heading-eyebrow {
  font-size: 9px;
  letter-spacing: 1.3px;
  color: var(--muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.heading-description {
  grid-column: 1;
  grid-row: 2;
  margin: 0;
  font-size: 11px;
  line-height: 1.6;
}
.shared-page-actions {
  grid-column: 2;
  grid-row: 1;
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
}
.has-description .shared-page-actions {
  grid-row: 1 / 3;
}
.shared-page-actions :deep(.button) {
  min-height: 32px;
  padding: 7px 11px;
  font-size: 11px;
}
@media (max-width: 760px) {
  .shared-page-heading {
    gap: 5px 10px;
    margin-bottom: 6px;
  }
  h1 {
    font-size: 21px;
  }
  .heading-eyebrow {
    display: none;
  }
  .heading-description {
    grid-column: 1 / -1;
  }
  .has-description .shared-page-actions {
    grid-row: 1;
  }
  .shared-page-actions {
    gap: 6px;
  }
  .shared-page-actions :deep(.button) {
    min-height: 32px;
    padding: 7px 9px;
    font-size: 10px;
  }
  .shared-page-heading.workflow-heading .shared-page-actions {
    grid-column: 1 / -1;
    grid-row: 2;
    justify-content: flex-start;
  }
  .shared-page-heading.workflow-heading .heading-description {
    display: none;
  }
  .shared-page-heading.workflow-heading :deep(.steps) {
    display: flex;
    gap: 8px;
    font-size: 10px;
  }
  .shared-page-heading.workflow-heading :deep(.steps i) {
    width: 17px;
    height: 17px;
    font-size: 10px;
  }
  .shared-page-heading.workflow-heading :deep(.steps > span) {
    gap: 5px;
  }
}
</style>
