<script setup lang="ts">
import { ref, watch } from 'vue'
import { Search, Tv, Film, Check } from '@lucide/vue'
import Button from '../../components/ui/Button.vue'
import Input from '../../components/ui/Input.vue'
import Select from '../../components/ui/Select.vue'
import EliText from '../../components/ui/EliText.vue'
import SidePanel from '../../components/ui/SidePanel.vue'
import type { Entry } from '../../model'
import type { ConfirmationGroup } from './useConfirmationGroup'
const props = defineProps<{ state: ConfirmationGroup; disabled: boolean; fileCount: number }>()
const s = props.state
const { entry, query, kind, searching, results } = s
const pickerOpen = ref(false)
watch(() => props.disabled, value => { if (value) pickerOpen.value = false })
function choose(work: Entry) {
  if (props.disabled) return
  pickerOpen.value = false
  void s.pick(work)
}
</script>
<template>
  <section class="confirm-work" aria-label="选择作品">
    <div class="confirm-work-heading">
      <div class="confirm-picked-work">
        <img v-if="entry?.posterPath" :src="`https://image.tmdb.org/t/p/w92${entry.posterPath}`" alt="" />
        <span v-else class="confirm-work-icon"><Film v-if="entry?.type === 'movie'" :size="22" /><Tv v-else :size="22" /></span>
        <div><h2><EliText :text="entry?.title || '待指定作品'" /></h2><span><template v-if="entry">{{ entry.year || '年份未知' }} · {{ entry.type === 'tv' ? '剧集' : '电影' }} · </template>{{ fileCount }} 个文件</span></div>
      </div>
      <Button variant="ghost" size="sm" :disabled="disabled" :aria-expanded="pickerOpen" @click="pickerOpen = true"><Search :size="14" />{{ entry ? '更换作品' : '选择作品' }}</Button>
      <slot name="actions" />
    </div>
    <SidePanel v-if="pickerOpen" v-model:open="pickerOpen" :title="entry ? '更换作品' : '选择作品'">
      <div class="confirm-work-picker">
        <form class="confirm-work-search" @submit.prevent="s.search">
          <Select v-model="kind" label="作品类型" :disabled="disabled || searching" :options="[{value:'tv',label:'剧集'},{value:'movie',label:'电影'}]" />
          <Input v-model="query" aria-label="搜索作品" placeholder="输入作品名称" :disabled="disabled || searching"><template #icon><Search :size="15" /></template></Input>
          <Button type="submit" :disabled="disabled || !query.trim()" :loading="searching">搜索</Button>
        </form>
        <div class="confirm-work-results" :aria-busy="searching">
          <Button v-for="work in results" :key="`${work.type}:${work.tmdbId}`" class="confirm-work-choice" :disabled="disabled || searching" :aria-pressed="entry?.type === work.type && entry.tmdbId === work.tmdbId" :aria-label="`${work.title} ${work.year || '年份未知'} · ${work.type === 'tv' ? '剧集' : '电影'} · TMDB ${work.tmdbId}`" @click="choose(work)">
            <img v-if="work.posterPath" :src="`https://image.tmdb.org/t/p/w92${work.posterPath}`" alt="" />
            <span v-else class="confirm-choice-icon"><Tv v-if="work.type === 'tv'" :size="18" /><Film v-else :size="18" /></span>
            <span class="confirm-choice-text"><strong>{{ work.title }}</strong><small>{{ work.year || '年份未知' }} · {{ work.type === 'tv' ? '剧集' : '电影' }} · TMDB {{ work.tmdbId }}</small></span>
            <Check v-if="entry?.type === work.type && entry.tmdbId === work.tmdbId" :size="16" />
          </Button>
          <p v-if="!results.length && !searching" class="muted">没有找到作品，请尝试其他名称</p>
        </div>
      </div>
      <template #footer><span>应用于本组已选 {{ s.chosen.value.length }} 个文件</span><Button @click="pickerOpen = false">取消</Button></template>
    </SidePanel>
  </section>
</template>
