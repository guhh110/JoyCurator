<script setup lang="ts">
import type { ExampleResult } from './useRuleExamples'
import EliText from '../../components/ui/EliText.vue'
defineProps<{
  results: ExampleResult[]
  checked: boolean
  busy: boolean
  stale: boolean
  error: string
}>()
</script>

<template>
  <section v-if="busy || checked || error" class="example-report" aria-label="保存前样例验证">
    <p v-if="busy" role="status">正在验证当前规则链及全部样例…</p>
    <p v-else-if="error" class="red" role="alert">{{ error }} · 尚未保存</p>
    <template v-else>
      <p :class="stale ? 'muted' : results.some(item => !item.passed) ? 'red' : 'green'" role="status">
        {{ stale ? '验证结果已过期，请重新验证' : results.length ? `样例 ${results.filter(item => item.passed).length}/${results.length} 通过` : '规则链校验通过，尚未配置测试样例' }}
        <span v-if="!stale && results.some(item => !item.passed)"> · 修正失败样例后可保存</span>
      </p>
      <details v-if="results.some(item => !item.passed)">
        <summary>查看失败样例</summary>
        <article v-for="(item, index) in results.filter(item => !item.passed)" :key="index">
          <EliText :text="item.path" />
          <p class="tiny muted">规则 {{ item.ruleId }} · 期望 {{ JSON.stringify(item.expected) }} · 实际 {{ JSON.stringify(item.actual) }}</p>
          <p v-if="item.error" class="tiny red">执行错误：{{ item.error }}</p>
        </article>
      </details>
    </template>
  </section>
</template>

<style scoped>
.example-report { border:1px solid var(--line); background:#fbfaff; border-radius:8px; padding:12px 14px; min-width:0; font-size:12px; }
p { margin:0; line-height:1.7; overflow-wrap:anywhere; }
summary { cursor:pointer; margin-top:8px; }
article { border-top:1px solid var(--line); margin-top:8px; padding-top:8px; min-width:0; }
</style>
