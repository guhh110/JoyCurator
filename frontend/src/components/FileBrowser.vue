<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  ArrowUp,
  ChevronRight,
  Folder,
  Film,
  HardDrive,
  LoaderCircle,
} from '@lucide/vue'
import Button from './ui/Button.vue'
import Input from './ui/Input.vue'
import Checkbox from './ui/Checkbox.vue'
import EliText from './ui/EliText.vue'
import { api, name } from '../api'
import Modal from './Modal.vue'
import { useToastFeedback } from '../app/toast'
const props = defineProps<{ directory?: boolean; confirmLabel?: string; scope?: 'target' }>()
const emit = defineEmits<{ close: []; select: [paths: string[]] }>()
const path = ref(''),
  typed = ref(''),
  items = ref<{ path: string; dir: boolean }[]>([]),
  selected = ref(new Set<string>()),
  loading = ref(false),
  error = ref('')
useToastFeedback(error)
const roots = ref<string[]>([])
const count = computed(() => selected.value.size)
async function open(next: string) {
  loading.value = true
  error.value = ''
  try {
    const result = await api<typeof items.value>(
      `/browse?path=${encodeURIComponent(next)}${props.scope ? `&scope=${props.scope}` : props.directory ? '&scope=roots' : ''}`,
    )
    if (!next) roots.value = result.map((item) => item.path)
    items.value = result.filter((item) => !props.directory || item.dir)
    path.value = next
    typed.value = next
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}
function toggle(p: string) {
  const next = new Set(selected.value)
  next.has(p) ? next.delete(p) : next.add(p)
  selected.value = next
}
function choose() {
  const paths = props.directory
    ? path.value
      ? [path.value]
      : []
    : [...selected.value]
  if (paths.length) emit('select', paths)
}
onMounted(() => open(''))
</script>
<template>
  <Modal
    :title="directory ? '选择目录' : '添加本地文件'"
    @close="emit('close')"
  >
    <p class="muted">
      {{
        directory
          ? '选择服务端已存在的目录。'
          : '勾选文件或整个文件夹，目录会递归展开为视频。'
      }}
    </p>
    <form class="path-bar" @submit.prevent="open(typed)">
      <Button
        type="button"
        variant="ghost" icon
        aria-label="上一级目录"
        :disabled="!path || loading"
        @click="
          open(
            roots.includes(path) ? '' : path.split('/').slice(0, -1).join('/'),
          )
        "
      >
        <ArrowUp :size="17" /></Button
      ><Input
        v-model="typed"
        aria-label="目录路径"
        :placeholder="scope === 'target' ? 'Target 目录路径' : '源目录路径'"
      /><Button type="submit" :disabled="loading">前往</Button>
    </form>
    <div class="browser-list">
      <div v-if="loading" class="empty small-empty">
        <LoaderCircle class="spin" />读取目录…
      </div>
      <div
        v-for="item in items"
        :key="item.path"
        class="browser-row"
        :class="{ picked: selected.has(item.path) }"
      >
        <Checkbox v-if="!directory && path" :label="`选择 ${name(item.path)}`" :model-value="selected.has(item.path)" @update:model-value="toggle(item.path)" />
        <component
          :is="!path ? HardDrive : item.dir ? Folder : Film"
          :size="19"
          :class="item.dir ? 'purple' : 'muted'"
        />
        <Button
          variant="ghost" class="browser-name"
          :title="item.path"
          @click="item.dir ? open(item.path) : toggle(item.path)"
        >
          <EliText :text="name(item.path)" ellipsis="start" />
        </Button>
        <Button
          v-if="item.dir"
          variant="ghost" icon
          :aria-label="`打开 ${name(item.path)}`"
          @click="open(item.path)"
        >
          <ChevronRight :size="17" />
        </Button>
      </div>
      <p v-if="!loading && error && !items.length" class="empty small-empty">
        未能读取目录，请检查路径后重试。
      </p>
      <p v-else-if="!loading && !items.length" class="empty small-empty">
        这个目录下没有{{ directory ? '子目录' : '可添加的视频' }}
      </p>
    </div>
    <template #footer
      ><span class="muted">{{
        directory ? (path ? name(path) : '请进入一个目录') : `已选 ${count} 项`
      }}</span
      ><Button
        variant="primary"
        :disabled="loading || (directory ? !path : !count)"
        @click="choose"
      >
        {{ confirmLabel || (directory ? '使用此目录' : '添加到工作台') }}
      </Button></template
    >
  </Modal>
</template>

<style scoped>
.path-bar>.ui-input-wrap{flex:1}.browser-name.ui-button{justify-content:flex-start;min-width:0;flex:1;height:auto;padding:8px 0;color:inherit;white-space:normal}
</style>
