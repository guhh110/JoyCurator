// 标注只读取词和同组相邻词；范围始终指向原文（UTF-16）。
function item(source, start, end, value, strength, reason, status) {
  return {
    value,
    spans: [{ sourceId: source.id, start, end }],
    strength: strength || 'explicit',
    status: status || 'valid',
    reason,
  }
}
function words(ctx, source) {
  return ctx.tokens
    .filter(
      (t) =>
        t.sourceId === source.id && (t.kind === 'text' || t.kind === 'number'),
    )
    .sort((a, b) => a.start - b.start)
}
function runs(ctx, source) {
  const parents = new Map(),
    result = []
  for (const t of ctx.tokens)
    if (t.sourceId === source.id) {
      const key = t.parentId,
        list = parents.get(key) || []
      list.push(t)
      parents.set(key, list)
    }
  for (const [parentId, list] of parents) {
    list.sort((a, b) => a.start - b.start)
    let sequence = []
    function flush() {
      if (sequence.length)
        result.push({
          parentId,
          words: sequence,
          start: sequence[0].start,
          end: sequence[sequence.length - 1].end,
        })
      sequence = []
    }
    for (const t of list) {
      if (t.kind === 'group') flush()
      else if (t.kind === 'text' || t.kind === 'number') sequence.push(t)
    }
    flush()
  }
  return result.sort((a, b) => a.start - b.start)
}
function overlaps(ctx, source, start, end, roles) {
  return ctx.candidates.some(
    (c) =>
      roles.includes(c.role) &&
      c.spans.some(
        (s) => s.sourceId === source.id && start < s.end && end > s.start,
      ),
  )
}
function scan(ctx, source, pattern, roles = []) {
  const result = []
  for (const run of runs(ctx, source)) {
    const chars = source.text.slice(run.start, run.end).split('')
    for (const c of ctx.candidates)
      if (roles.includes(c.role))
        for (const s of c.spans)
          if (s.sourceId === source.id)
            for (
              let i = Math.max(s.start, run.start);
              i < Math.min(s.end, run.end);
              i++
            )
              chars[i - run.start] = ' '
    for (const m of chars.join('').matchAll(pattern)) {
      m.index += run.start
      result.push(m)
    }
  }
  return result.sort((a, b) => a.index - b.index)
}
const technical = [
  'resolution',
  'videoCodec',
  'audioCodec',
  'bitDepth',
  'dynamicRange',
  'language',
  'subtitleLanguage',
  'checksum',
  'releaseTag',
]
function isYear(n) {
  return n >= 1900 && n <= 2099
}

function extract(ctx) {
  const out = []
  for (const s of ctx.sources) {
    if (
      s.kind === 'directories' &&
      /^(?:specials?|特别篇|特別篇)$/i.test(
        words(ctx, s)
          .map((t) => t.text)
          .join(' '),
      )
    )
      continue
    const occupied = ctx.candidates
        .filter((c) => c.role !== 'title' &&
          !(c.role === 'episodes' && c.strength === 'heuristic' && c.status === 'valid'))
        .flatMap((c) => c.spans.filter((p) => p.sourceId === s.id))
        .sort((a, b) => a.start - b.start),
      root = [],
      grouped = []
    for (const run of runs(ctx, s)) {
      const parts = []
      for (const t of run.words) {
        let pos = t.start
        for (const p of occupied) {
          if (p.end <= pos || p.start >= t.end) continue
          if (p.start > pos) parts.push({ start: pos, end: p.start })
          pos = Math.max(pos, p.end)
          if (pos >= t.end) break
        }
        if (pos < t.end) parts.push({ start: pos, end: t.end })
      }
      let sequence = []
      function flush() {
        if (sequence.length) {
          const start = sequence[0].start,
            end = sequence[sequence.length - 1].end,
            value = sequence.map((p) => s.text.slice(p.start, p.end)).join(' '),
            target = run.parentId === null ? root : grouped
          target.push(
            item(
              s,
              start,
              end,
              value,
              'contextual',
              run.parentId === null ? '来源片名' : '括号片名',
            ),
          )
        }
        sequence = []
      }
      for (const p of parts) {
        const previous = sequence[sequence.length - 1]
        if (
          previous &&
          occupied.some((x) => x.start < p.start && x.end > previous.end)
        )
          flush()
        sequence.push(p)
      }
      flush()
    }
    out.push(...(root.length ? root : grouped))
  }
  return out
}
