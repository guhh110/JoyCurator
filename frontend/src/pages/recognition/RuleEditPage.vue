<script setup lang="ts">
import RuleEdit from '../../features/extraction/RuleEdit.vue'
defineProps<{
  ruleId: string
  target?: string
  kind?: string
  template?: string
  line?: string
}>()
</script>
<template>
  <RuleEdit
    :key="ruleId"
    :rule-id="ruleId"
    :target="target"
    :kind="kind"
    :template="template"
    :line="line"
  />
</template>
