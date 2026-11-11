<script setup lang="ts">
import { ref, computed } from 'vue'
import Button from '../../components/ui/Button.vue'
import Input from '../../components/ui/Input.vue'
import Field from '../../components/ui/Field.vue'
import Checkbox from '../../components/ui/Checkbox.vue'
import EliText from '../../components/ui/EliText.vue'
import type { MediaLayout, RecognitionItem } from '../../model'
const props = defineProps<{ files: RecognitionItem[]; busy: boolean }>()
const emit = defineEmits<{ save: [changes: { paths: string[]; layout: MediaLayout }[]] }>()
const rows = ref([...new Set(props.files.map(file => `${file.season}:${file.path.slice(0, file.path.lastIndexOf('/'))}`))].map(key => {
  const files = props.files.filter(file => `${file.season}:${file.path.slice(0, file.path.lastIndexOf('/'))}` === key)
  const first = files[0]!
  return { key, files, workDir: first.layout?.workDir || first.path.slice(0, first.path.lastIndexOf('/')), seasonDir: first.layout?.seasonDir || '', moviePrefix: first.layout?.moviePrefix ?? false }
}))
const movie = computed(() => props.files[0]?.entry?.type === 'movie')
function save() { emit('save', rows.value.map(row => ({ paths: row.files.map(file => file.path), layout: { workDir: row.workDir.trim(), seasonDir: movie.value ? '' : row.seasonDir.trim(), moviePrefix: movie.value && row.moviePrefix, manual: true } }))) }
</script>
<template>
  <div class="flow-layout-editor">
    <section v-for="row in rows" :key="row.key" class="flow-layout-row">
      <EliText :text="row.files[0]!.path.slice(0, row.files[0]!.path.lastIndexOf('/'))" ellipsis="start" />
      <Field label="作品目录"><Input v-model="row.workDir" aria-label="作品目录" :disabled="busy" /></Field>
      <Field v-if="!movie" label="季目录"><Input v-model="row.seasonDir" placeholder="平铺目录留空" aria-label="季目录" :disabled="busy" /></Field>
      <label v-else class="flow-checkbox"><Checkbox :model-value="row.moviePrefix" label="图片使用视频文件名前缀" :disabled="busy" @update:model-value="row.moviePrefix = $event === true" />图片使用视频文件名前缀</label>
    </section>
    <Button :disabled="busy || rows.some(row => !row.workDir.trim())" @click="save">保存输出目录</Button>
  </div>
</template>
