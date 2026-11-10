<script setup lang="ts">
import type { Stage } from './model'
defineProps<{ label: string; editorId: string; stage: Stage }>()
</script>
<template>
  <div>
    <label :for="editorId">{{ label }}</label
    ><slot />
    <details>
      <summary>脚本接口</summary>
      <p v-if="stage === 'tokenize'">
        输入 ctx.sources；返回片段数组：id、sourceId、parentId、kind（text /
        number / separator / group）、text、start、end。位置为
        UTF-16，原文不得丢失。默认按非中英文数字字符分隔，第3集等混合词保持完整。
      </p>
      <p v-else-if="stage === 'annotate'">
        输入
        ctx.file、sources、tokens、candidates、params；返回候选数组：value、spans（sourceId
        / start / end）、strength（explicit / contextual /
        heuristic）、status（valid / invalid）、reason。ID、ruleId、role
        由系统填写。按 tokens 的词语及同组相邻词标注，可引用词内子范围。
        只使用本规则所选来源。valid 表示值合法，不等于采纳。片名最后标注，保留推测集数文字。
      </p>
      <p v-else>
        输入
        ctx.contextRoot、sourceBoundary、tree、files、targetFileIds、params。files
        包含整个目录树的来源、片段与候选。为每个目标返回
        fileId、metadata、decisions；非空字段需引用 candidates（fileId /
        candidateId），可跨文件引用。推测集数需明确季集依据与目录数字变化佐证；裁剪片名时同时引用片名和集数依据。
      </p>
      <p>
        入口 function extract(ctx)。输入只读，所有位置均为 UTF-16。未知标量用
        null，未知集数用 []，0 为有效季集号。可用
        trace、print、reSearch、reFindAll、reSub；无文件与网络访问。
      </p>
      <p>
        分词与标注 250 ms；裁决 5 s、输入 64 MiB、输出 16 MiB；目录最多 2000
        个视频。分词每文件最多 1024 片段、分组 16 层；每规则最多 512
        候选、每文件最多 1024 候选。
      </p>
    </details>
  </div>
</template>
<style scoped>
details {
  font-size: 12px;
  line-height: 1.8;
  margin-top: 10px;
}
summary {
  cursor: pointer;
  color: var(--purple);
}
</style>
