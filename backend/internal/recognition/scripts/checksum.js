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
  for (const s of ctx.sources)
    for (const t of words(ctx, s)) {
      if (!/^[A-Fa-f0-9]{8}$/.test(t.text)) continue
      const siblings = words(ctx, s).filter((w) => w.parentId === t.parentId)
      if (siblings.length === 1)
        out.push(item(s, t.start, t.end, t.text, 'explicit', '技术标签'))
    }
  return out
}
