// 整棵目录树一次裁决；所有非空字段必须引用真实候选。
function extract(ctx) {
  const rank = { explicit: 0, contextual: 1, heuristic: 2 },
    byId = new Map(ctx.files.map((f) => [f.fileId, f]))
  const sourceDepth = (f, c) =>
    Math.min(
      ...c.spans.map((p) => f.sources.find((s) => s.id === p.sourceId).depth),
    )
  const norm = (s) => (s || '').toLowerCase().replace(/[ ._-]/g, '')
  function ordered(f, role) {
    return f.candidates
      .filter((c) => c.role === role)
      .slice()
      .sort(
        (a, b) =>
          rank[a.strength] - rank[b.strength] ||
          sourceDepth(f, a) - sourceDepth(f, b) ||
          a.spans[0].start - b.spans[0].start,
      )
  }
  function first(f, role) {
    const list = ordered(f, role)
    if (role === 'title') {
      const names = list.filter(
          (c) => c.status === 'valid' && sourceDepth(f, c) === 0,
        ),
        folders = list.filter(
          (c) => c.status === 'valid' && sourceDepth(f, c) > 0,
        )
      if (names.length > 1) {
        const matched = names.find((c) =>
          folders.some((d) => norm(d.value) === norm(c.value)),
        )
        if (matched) return matched
      }
    }
    const c = list[0]
    return c && c.status === 'valid' ? c : null
  }
  const titles = new Map(ctx.files.map((f) => [f.fileId, first(f, 'title')]))
  function signature(f, c) {
    const p = c.spans[0],
      s = f.sources.find((s) => s.id === p.sourceId),
      ranges = [{ start: p.start, end: p.end, label: '#episode' }]
    for (const candidate of f.candidates)
      if (
        [
          'resolution',
          'videoCodec',
          'audioCodec',
          'bitDepth',
          'dynamicRange',
          'language',
          'subtitleLanguage',
          'checksum',
          'releaseTag',
        ].includes(candidate.role)
      )
        for (const span of candidate.spans)
          if (span.sourceId === p.sourceId)
            ranges.push({ ...span, label: '#' + candidate.role })
    ranges.sort((a, b) => a.start - b.start)
    let text = '',
      end = 0
    for (const r of ranges) {
      if (r.start < end) continue
      text += s.text.slice(end, r.start) + r.label
      end = r.end
    }
    return text + s.text.slice(end)
  }
  const signatures = new Map(),
    local = new Map()
  for (const f of ctx.files) {
    const title = titles.get(f.fileId),
      work = f.directory + '|' + norm(title && title.value),
      peers = local.get(work) || []
    peers.push(f)
    local.set(work, peers)
    for (const c of f.candidates)
      if (
        c.role === 'episodes' &&
        c.status === 'valid' &&
        c.strength === 'heuristic'
      ) {
        const key = work + '|' + signature(f, c)
        const list = signatures.get(key) || []
        list.push({ f, c })
        signatures.set(key, list)
      }
  }
  function varying(f, c) {
    const title = titles.get(f.fileId),
      key =
        f.directory + '|' + norm(title && title.value) + '|' + signature(f, c)
    const list = signatures.get(key) || []
    return new Set(list.map((x) => JSON.stringify(x.c.value))).size > 1
      ? list
      : []
  }
  const results = []
  for (const id of ctx.targetFileIds) {
    const f = byId.get(id),
      metadata = {
        tmdbId: null,
        title: null,
        mediaType: null,
        year: null,
        season: null,
        episodes: [],
        releaseTag: null,
      },
      decisions = {}
    function set(role, c, file = f, reason) {
      metadata[role] = c.value
      decisions[role] = {
        candidates: [{ fileId: file.fileId, candidateId: c.id }],
        reason: reason || c.reason,
      }
    }
    for (const role of [
      'tmdbId',
      'title',
      'year',
      'season',
      'releaseTag',
      'mediaType',
    ]) {
      const c = first(f, role)
      if (c) set(role, c)
    }
    const ep = ordered(f, 'episodes')
    if (ep.length && ep[0].status === 'valid') {
      const c = ep[0]
      if (c.strength !== 'heuristic') set('episodes', c)
      else {
        const varyingCandidates = ep.filter(
          (c) => c.status === 'valid' && varying(f, c).length,
        )
        const choices = varyingCandidates.length
          ? varyingCandidates
          : ep.length === 1 && metadata.season !== null
            ? [c]
            : []
        if (choices.length === 1) {
          const picked = choices[0],
            peers = varying(f, picked)
          set(
            'episodes',
            picked,
            f,
            peers.length ? '同一位置数字随文件变化' : '季标记下唯一括号集号',
          )
          if (peers.length)
            decisions.episodes.candidates.push(
              ...peers
                .filter((p) => p.f.fileId !== id)
                .slice(0, 3)
                .map((p) => ({ fileId: p.f.fileId, candidateId: p.c.id })),
            )
        }
      }
    }
    if (
      metadata.season === null &&
      metadata.title !== null &&
      !f.candidates.some((c) => c.role === 'season' && c.status === 'invalid')
    ) {
      const peers = local.get(f.directory + '|' + norm(metadata.title)) || [],
        seasons = peers
          .map((file) => ({ file, c: first(file, 'season') }))
          .filter((x) => x.c)
      if (seasons.length && new Set(seasons.map((x) => x.c.value)).size === 1)
        set('season', seasons[0].c, seasons[0].file, '同目录同片名唯一明确季号')
    }
    if (
      metadata.mediaType === null &&
      (metadata.season !== null || metadata.episodes.length)
    ) {
      metadata.mediaType = 'tv'
      decisions.mediaType = {
        candidates: [
          ...(decisions.season?.candidates || []),
          ...(decisions.episodes?.candidates || []),
        ],
        reason: '季号或集数提供剧集依据',
      }
    }
    results.push({ fileId: id, metadata, decisions })
  }
  trace('裁决完成', { contextFiles: ctx.files.length, targets: results.length })
  return results
}
