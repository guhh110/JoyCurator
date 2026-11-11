// 先确定季集，再从保留原文的片名候选中排除已确认的集数。
function extract(ctx) {
  const rank = { explicit: 0, contextual: 1, heuristic: 2 },
    states = new Map()
  const norm = (s) => (s || '').toLowerCase().replace(/[\s._-]/g, '')
  const ref = (f, c) => ({ fileId: f.fileId, candidateId: c.id })
  const unique = (refs) => [...new Map(refs.map((r) =>
    [r.fileId + '/' + r.candidateId, r])).values()]
  const depth = (f, c) => Math.min(...c.spans.map((p) =>
    f.sources.find((s) => s.id === p.sourceId).depth))
  const compare = (f, a, b) => rank[a.strength] - rank[b.strength] ||
    depth(f, a) - depth(f, b) || a.spans[0].start - b.spans[0].start ||
    a.ruleId.localeCompare(b.ruleId) || a.id.localeCompare(b.id)
  const overlap = (a, b) => a.spans.some((x) => b.spans.some((y) =>
    x.sourceId === y.sourceId && x.start < y.end && x.end > y.start))
  function eligible(f, c) {
    const ids = f.candidates.filter((x) => x.role === 'tmdbId')
    if (['season', 'episodes', 'year'].includes(c.role) && ids.some((x) => overlap(c, x)))
      return false
    if (c.role === 'year' && f.candidates.some((x) =>
      ['season', 'episodes'].includes(x.role) && x.strength !== 'heuristic' &&
      !ids.some((id) => overlap(x, id)) && overlap(c, x))) return false
    if (c.role === 'episodes' && c.strength === 'heuristic' &&
        f.candidates.some((x) => ['season', 'year'].includes(x.role) &&
          eligible(f, x) && overlap(c, x))) return false
    return true
  }
  const ordered = (f, role) => f.candidates.filter((c) => c.role === role && eligible(f, c))
    .slice().sort((a, b) => compare(f, a, b))
  function first(f, role) {
    const c = ordered(f, role)[0]
    return c && c.status === 'valid' ? c : null
  }
  function set(state, role, c, f, reason) {
    state.metadata[role] = c.value
    state.decisions[role] = { candidates: [ref(f, c)], reason: reason || c.reason }
  }
  for (const f of ctx.files) {
    const state = { metadata: { tmdbId: null, title: null, mediaType: null,
      year: null, season: null, episodes: [] }, decisions: {},
      episode: null, blocked: f.candidates.some((c) => c.role === 'episodes' &&
        c.strength !== 'heuristic' && c.status === 'invalid' && eligible(f, c)) }
    for (const role of ['tmdbId', 'year', 'season', 'mediaType']) {
      const c = first(f, role)
      if (c) set(state, role, c, f)
    }
    const c = first(f, 'episodes')
    if (c && c.strength !== 'heuristic') {
      state.episode = c
      set(state, 'episodes', c, f)
    }
    states.set(f.fileId, state)
  }
  function signature(f, c) {
    if (c.spans.some((p) => p.sourceId !== 'name')) return null
    const source = f.sources.find((s) => s.id === 'name'),
      ranges = c.spans.map((p) => ({ ...p, label: '#episode', priority: 0 }))
    for (const other of f.candidates) {
      if (other.status !== 'valid' || other.role !== 'season' || !eligible(f, other)) continue
      for (const p of other.spans)
        if (p.sourceId === 'name' && !c.spans.some((x) =>
          p.start < x.end && p.end > x.start))
          ranges.push({ ...p, label: '',
            priority: 1 })
    }
    ranges.sort((a, b) => a.start - b.start || a.priority - b.priority || b.end - a.end)
    let text = '', end = 0
    for (const p of ranges) {
      if (p.start < end) continue
      text += source.text.slice(end, p.start) + p.label
      end = p.end
    }
    return norm(text + source.text.slice(end))
  }
  // 使用命名结构建立索引，避免依赖尚未确定的片名或两两比较文件。
  const buckets = new Map()
  for (const f of ctx.files) {
    const state = states.get(f.fileId)
    if (state.blocked) continue
    const list = state.episode ? [state.episode] : ordered(f, 'episodes')
      .filter((c) => c.status === 'valid' && c.strength === 'heuristic')
    for (const c of list) {
      const shape = signature(f, c)
      if (shape === null) continue
      const key = JSON.stringify([f.directory, shape]), nodes = buckets.get(key) || []
      nodes.push({ f, c, state, value: JSON.stringify(c.value) })
      buckets.set(key, nodes)
    }
  }
  const supported = new Map()
  for (const nodes of buckets.values()) {
    const seasons = new Set(nodes.map((n) => n.state.metadata.season).filter((v) => v !== null)),
      ids = new Set(nodes.map((n) => n.state.metadata.tmdbId).filter((v) => v !== null)),
      season = seasons.size === 1 ? [...seasons][0] : null,
      id = ids.size === 1 ? [...ids][0] : null,
      groups = new Map()
    for (const n of nodes) {
      // 无身份的文件仅在组内身份唯一时参与，不能连接不同作品或季。
      const key = JSON.stringify([n.state.metadata.season ?? season,
        n.state.metadata.tmdbId ?? id]), group = groups.get(key) || []
      group.push(n)
      groups.set(key, group)
    }
    for (const group of groups.values()) {
      group.sort((a, b) => a.f.path < b.f.path ? -1 : a.f.path > b.f.path ? 1 :
        a.c.spans[0].start - b.c.spans[0].start || a.c.id.localeCompare(b.c.id))
      const anchor = group.find((n) => n.state.metadata.season !== null ||
        n.c.strength !== 'heuristic')
      if (!anchor) continue
      // 每个值保留两个不同文件，提供有界且可重建的不同集号佐证。
      const values = new Map()
      for (const n of group) {
        const representatives = values.get(n.value) || []
        if (representatives.length < 2 && !representatives.some((x) => x.f.fileId === n.f.fileId))
          representatives.push(n)
        values.set(n.value, representatives)
      }
      if (values.size < 2) continue
      const representatives = [...values.values()].slice(0, 2).flat(),
        anchorCandidate = anchor.state.metadata.season !== null
          ? first(anchor.f, 'season') : anchor.c
      for (const n of group) {
        if (n.c.strength !== 'heuristic') continue
        const peer = representatives.find((x) => x.value !== n.value && x.f.fileId !== n.f.fileId)
        if (!peer) continue
        const choices = supported.get(n.f.fileId) || new Map(),
          key = JSON.stringify([n.c.spans, n.c.value])
        if (!choices.has(key)) choices.set(key, { c: n.c, refs: unique([
          ref(n.f, n.c), ref(peer.f, peer.c), ref(anchor.f, anchorCandidate)]) })
        supported.set(n.f.fileId, choices)
      }
    }
  }
  for (const f of ctx.files) {
    const state = states.get(f.fileId), choices = supported.get(f.fileId)
    if (state.episode || state.blocked || !choices || choices.size !== 1) continue
    const picked = [...choices.values()][0]
    state.episode = picked.c
    state.metadata.episodes = picked.c.value
    state.decisions.episodes = { candidates: picked.refs,
      reason: '明确季集依据下，同目录唯一数字位置随文件变化' }
  }
  function titles(f, episode) {
    const output = []
    for (const c of ordered(f, 'title')) {
      if (c.status !== 'valid' || !episode || !c.spans.some((p) =>
        episode.spans.some((x) => x.sourceId === p.sourceId && x.start < p.end && x.end > p.start))) {
        output.push(c)
        continue
      }
      for (const p of c.spans) {
        const cuts = episode.spans.filter((x) => x.sourceId === p.sourceId)
          .sort((a, b) => a.start - b.start), remaining = []
        let start = p.start
        for (const cut of cuts) {
          if (cut.end <= start || cut.start >= p.end) continue
          if (cut.start > start) remaining.push({ start, end: cut.start })
          start = Math.max(start, cut.end)
        }
        if (start < p.end) remaining.push({ start, end: p.end })
        const source = f.sources.find((s) => s.id === p.sourceId)
        for (const part of remaining) {
          // 按原有词及括号边界取剩余区段，不跨被移除的数字拼接。
          let words = [], parent
          function flush() {
            if (words.length) output.push({ ...c, trimmed: true,
              value: words.map((w) => source.text.slice(w.start, w.end)).join(' '),
              spans: [{ sourceId: p.sourceId, start: words[0].start,
                end: words[words.length - 1].end }] })
            words = []
          }
          for (const t of f.tokens) {
            if (t.sourceId !== p.sourceId || t.end <= part.start || t.start >= part.end) continue
            if (t.kind === 'group') { flush(); continue }
            if (t.kind !== 'text' && t.kind !== 'number') continue
            if (words.length && t.parentId !== parent) flush()
            parent = t.parentId
            words.push({ start: Math.max(t.start, part.start), end: Math.min(t.end, part.end) })
          }
          flush()
        }
      }
    }
    return output.sort((a, b) => compare(f, a, b))
  }
  for (const f of ctx.files) {
    const state = states.get(f.fileId), list = titles(f, state.episode),
      names = list.filter((c) => c.status === 'valid' && depth(f, c) === 0),
      folders = list.filter((c) => c.status === 'valid' && depth(f, c) > 0),
      matched = names.length > 1 && names.find((c) =>
        folders.some((d) => norm(d.value) === norm(c.value))),
      c = matched || list[0]
    if (c && c.status === 'valid') {
      set(state, 'title', c, f, c.trimmed ? '片名排除目录确认的集数' : c.reason)
      if (c.trimmed) state.decisions.title.candidates = unique([
        ...state.decisions.title.candidates, ...state.decisions.episodes.candidates])
    }
  }
  const local = new Map()
  for (const f of ctx.files) {
    const m = states.get(f.fileId).metadata
    if (m.title === null || m.season === null) continue
    const key = JSON.stringify([f.directory, norm(m.title)]), peers = local.get(key) || []
    peers.push(f)
    local.set(key, peers)
  }
  for (const f of ctx.files) {
    const state = states.get(f.fileId), m = state.metadata
    if (m.season === null && m.title !== null &&
        !f.candidates.some((c) => c.role === 'season' && c.status === 'invalid' && eligible(f, c))) {
      const peers = (local.get(JSON.stringify([f.directory, norm(m.title)])) || [])
        .filter((p) => m.tmdbId === null || states.get(p.fileId).metadata.tmdbId === null ||
          m.tmdbId === states.get(p.fileId).metadata.tmdbId),
        ids = new Set(peers.map((p) => states.get(p.fileId).metadata.tmdbId).filter((v) => v !== null)),
        seasons = new Set(peers.map((p) => states.get(p.fileId).metadata.season))
      if (peers.length && ids.size <= 1 && seasons.size === 1)
        set(state, 'season', first(peers[0], 'season'), peers[0], '同目录同片名唯一明确季号')
    }
    if (m.mediaType === null && (m.season !== null || m.episodes.length)) {
      m.mediaType = 'tv'
      state.decisions.mediaType = { candidates: unique([
        ...(state.decisions.season?.candidates || []),
        ...(state.decisions.episodes?.candidates || [])]), reason: '季号或集数提供剧集依据' }
    }
  }
  const results = ctx.targetFileIds.map((fileId) => ({ fileId,
    metadata: states.get(fileId).metadata, decisions: states.get(fileId).decisions }))
  trace('裁决完成', { contextFiles: ctx.files.length, targets: results.length })
  return results
}
