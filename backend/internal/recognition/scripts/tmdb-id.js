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
function scan(ctx, source, pattern) {
  const result = []
  for (const run of runs(ctx, source))
    for (const m of source.text.slice(run.start, run.end).matchAll(pattern)) {
      m.index += run.start
      result.push(m)
    }
  return result.sort((a, b) => a.index - b.index)
}

function extract(ctx) {
  const out = []
  for (const s of ctx.sources)
    for (const m of scan(ctx, s, /tmdb[-= ]?(\d+)/gi)) {
      const n = Number(m[1]),
        valid = n > 0 && n <= 999999999
      out.push(
        item(
          s,
          m.index,
          m.index + m[0].length,
          n,
          'explicit',
          valid ? 'TMDB 标记' : 'tmdbId 提取值类型或范围无效',
          valid ? 'valid' : 'invalid',
        ),
      )
    }
  return out
}
