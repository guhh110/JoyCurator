import { computed, ref, watch, onScopeDispose, type Ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api } from '../../api'
import type { Episode, Season, RecognitionItem } from '../../model'
import type { recognitionGroups } from '../../recognition'
import type { Runtime } from '../../app/runtime'

type Group = ReturnType<typeof recognitionGroups>[number]
export function useSeasonComparison(options: {
  current: Readonly<Ref<Group | undefined>>
  active: Ref<boolean>
  locked: Readonly<Ref<boolean>>
  runtime: Runtime
  act: (fn: () => Promise<void>) => Promise<void>
  save: () => Promise<void>
}) {
  const { current, active, locked, runtime, act, save } = options
  let alive = true
  onScopeDispose(() => { alive = false; seasonRequest++ })
  const comparisonSeason = ref<number>(1)
  const groupSeasons = ref<Season[]>([])
  const groupSeasonsLoading = ref(false)
  const groupSeasonsError = ref('')
  const detailView = ref<'files' | 'episodes'>('files')
  let seasonRequest = 0
  watch(() => current.value?.entry?.type === 'tv' ? current.value.entry.tmdbId : null, loadGroupSeasons, { immediate: true })
  async function loadGroupSeasons(id: number | null) {
    const request = ++seasonRequest
    groupSeasons.value = []
    groupSeasonsError.value = ''
    comparisonSeason.value = -1
    groupSeasonsLoading.value = !!id
    if (!id) return
    try {
      const value = await runtime.client.fetchQuery({
        queryKey: ['tmdb', 'seasons', id],
        queryFn: ({ signal }) => api<Season[]>(`/seasons?id=${id}`, undefined, 'GET', { signal }),
        staleTime: 60_000,
      })
      if (!alive || request !== seasonRequest) return
      groupSeasons.value = value
      const existing = current.value?.files[0]?.season
      comparisonSeason.value = value.find((season) => season.number === existing)?.number ?? value[0]?.number ?? -1
    } catch (e) {
      if (alive && request === seasonRequest) groupSeasonsError.value = (e as Error).message
    } finally {
      if (request === seasonRequest) groupSeasonsLoading.value = false
    }
  }
  const episodeQuery = useQuery({
    queryKey: computed(() => ['tmdb', 'episodes', current.value?.entry?.tmdbId, comparisonSeason.value]),
    enabled: computed(() => active.value && current.value?.entry?.type === 'tv' && comparisonSeason.value >= 0 && detailView.value === 'episodes'),
    queryFn: ({ signal }) => api<Episode[]>(`/episodes?id=${current.value!.entry!.tmdbId}&season=${comparisonSeason.value}`, undefined, 'GET', { signal }),
    staleTime: 60_000,
  })
  async function retryComparison() {
    if (groupSeasonsError.value) {
      await loadGroupSeasons(current.value?.entry?.type === 'tv' ? current.value.entry.tmdbId : null)
    } else if (comparisonSeason.value >= 0) {
      await episodeQuery.refetch()
    }
  }
  function viewSeason(season: number) {
    if (current.value?.entry?.type !== 'tv' || groupSeasonsLoading.value || !groupSeasons.value.some(item => item.number === season)) return
    comparisonSeason.value = season
  }
  async function assignEpisode(file: RecognitionItem, number: number) {
    if (locked.value || file.execution?.locked || file.execution?.completed || !current.value?.files.includes(file)) return
    if (!episodeQuery.data.value?.some(episode => episode.number === number)) return
    const season = comparisonSeason.value
    await act(async () => {
      file.season = season
      file.episodes = [number]
      file.selected = true
      await save()
    })
  }
  return { comparisonSeason, groupSeasons, groupSeasonsLoading, groupSeasonsError, detailView, episodeQuery, retryComparison, viewSeason, assignEpisode }
}
