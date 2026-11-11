<script setup lang="ts">
import { ref, watch } from 'vue'
import { Search, Tv, Film, Check } from '@lucide/vue'
import Button from '../../components/ui/Button.vue'
import Input from '../../components/ui/Input.vue'
import Select from '../../components/ui/Select.vue'
import type { useConfirmation } from './useConfirmation'
const props = defineProps<{ state: ReturnType<typeof useConfirmation>; disabled: boolean }>()
const s = props.state
const { entry, query, kind, searching, fetchingSeasons, results } = s
const searchOpen = ref(false)
watch(() => `${entry.value?.type}:${entry.value?.tmdbId}`, () => { searchOpen.value = false })
</script>
<template>
  <section class="confirm-work" aria-label="选择作品">
    <div class="confirm-work-heading">
      <div v-if="entry" class="confirm-picked-work">
        <img v-if="entry.posterPath" :src="`https://image.tmdb.org/t/p/w92${entry.posterPath}`" alt="" />
        <span v-else class="confirm-work-icon"><Tv v-if="entry.type === 'tv'" :size="22" /><Film v-else :size="22" /></span>
        <div><h2>{{ entry.title }}</h2><span>{{ entry.year || '年份未知' }} · {{ entry.type === 'tv' ? '剧集' : '电影' }} · TMDB {{ entry.tmdbId }}</span></div>
      </div>
      <h2 v-else>选择作品</h2>
      <Button variant="ghost" size="sm" :disabled="disabled" :aria-expanded="searchOpen" @click="searchOpen = !searchOpen"><Search :size="14" />{{ entry ? '更换作品' : '搜索作品' }}</Button>
    </div>
    <form v-if="searchOpen || !results.length" class="confirm-work-search" @submit.prevent="s.search">
      <Select v-model="kind" label="作品类型" :disabled="disabled || searching" :options="[{value:'tv',label:'剧集'},{value:'movie',label:'电影'}]" />
      <Input v-model="query" aria-label="搜索作品" placeholder="输入作品名称" :disabled="disabled || searching"><template #icon><Search :size="15" /></template></Input>
      <Button type="submit" :disabled="disabled || !query.trim()" :loading="searching">搜索</Button>
    </form>
    <div v-if="!entry || searchOpen" class="confirm-work-results">
      <Button v-for="work in results" :key="`${work.type}:${work.tmdbId}`" class="confirm-work-choice" :disabled="disabled || fetchingSeasons" :aria-label="`${work.title} ${work.year || '年份未知'} · ${work.type === 'tv' ? '剧集' : '电影'} · TMDB ${work.tmdbId}`" @click="s.pick(work)">
        <img v-if="work.posterPath" :src="`https://image.tmdb.org/t/p/w92${work.posterPath}`" alt="" />
        <span v-else class="confirm-choice-icon"><Tv v-if="work.type === 'tv'" :size="18" /><Film v-else :size="18" /></span>
        <span class="confirm-choice-text"><strong>{{ work.title }}</strong><small>{{ work.year || '年份未知' }} · {{ work.type === 'tv' ? '剧集' : '电影' }} · TMDB {{ work.tmdbId }}</small></span>
        <Check v-if="entry?.type === work.type && entry.tmdbId === work.tmdbId" :size="16" />
      </Button>
      <p v-if="!results.length && !searching" class="muted">没有找到作品，请尝试其他名称</p>
    </div>
  </section>
</template>
