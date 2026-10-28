<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  ArrowUp,
  Check,
  ChevronRight,
  Folder,
  Film,
  HardDrive,
  LoaderCircle,
} from '@lucide/vue'
import { api, name } from '../api'
import Modal from './Modal.vue'
const props = defineProps<{ directory?: boolean; confirmLabel?: string }>()
const emit = defineEmits<{ close: []; select: [paths: string[]] }>()
const path = ref(''),
  typed = ref(''),
  items = ref<{ path: string; dir: boolean }[]>([]),
  selected = ref(new Set<string>()),
  loading = ref(false),
  error = ref('')
const roots = ref<string[]>([])
const count = computed(() => selected.value.size)
async function open(next: string) {
  loading.value = true
  error.value = ''
  try {
    const result = await api<typeof items.value>(
      `/browse?path=${encodeURIComponent(next)}${props.directory ? '&scope=roots' : ''}`,
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
      <button
        type="button"
        class="icon-button"
        aria-label="上一级目录"
        :disabled="!path || loading"
        @click="
          open(
            roots.includes(path) ? '' : path.split('/').slice(0, -1).join('/'),
          )
        "
      >
        <ArrowUp :size="17" /></button
      ><input
        v-model="typed"
        aria-label="目录路径"
        placeholder="源目录路径"
      /><button class="button small" :disabled="loading">前往</button>
    </form>
    <p v-if="error" role="alert" class="notice error">{{ error }}</p>
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
        <button
          v-if="!directory && path"
          class="select-box"
          :aria-label="`选择 ${name(item.path)}`"
          :aria-pressed="selected.has(item.path)"
          @click="toggle(item.path)"
        >
          <Check v-if="selected.has(item.path)" :size="13" />
        </button>
        <component
          :is="!path ? HardDrive : item.dir ? Folder : Film"
          :size="19"
          :class="item.dir ? 'purple' : 'muted'"
        />
        <button
          class="browser-name"
          :title="item.path"
          @click="item.dir ? open(item.path) : toggle(item.path)"
        >
          {{ name(item.path) }}
        </button>
        <button
          v-if="item.dir"
          class="icon-button"
          :aria-label="`打开 ${name(item.path)}`"
          @click="open(item.path)"
        >
          <ChevronRight :size="17" />
        </button>
      </div>
      <p v-if="!loading && !items.length" class="empty small-empty">
        这个目录下没有{{ directory ? '子目录' : '可添加的视频' }}
      </p>
    </div>
    <template #footer
      ><span class="muted">{{
        directory ? (path ? name(path) : '请进入一个目录') : `已选 ${count} 项`
      }}</span
      ><button
        class="button primary"
        :disabled="loading || (directory ? !path : !count)"
        @click="choose"
      >
        {{ directory ? '使用此目录' : confirmLabel || '添加到工作台' }}
      </button></template
    >
  </Modal>
</template>
