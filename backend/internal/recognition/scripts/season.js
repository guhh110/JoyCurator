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
function scan(ctx, source, pattern) {
  const result = []
  for (const run of runs(ctx, source))
    for (const m of source.text.slice(run.start, run.end).matchAll(pattern)) {
      m.index += run.start
      result.push(m)
    }
  return result.sort((a, b) => a.index - b.index)
}
function number(text) {
  if (/^\d+$/.test(text)) return Number(text)
  const digits = {
      零: 0,
      〇: 0,
      一: 1,
      二: 2,
      两: 2,
      三: 3,
      四: 4,
      五: 5,
      六: 6,
      七: 7,
      八: 8,
      九: 9,
    },
    units = { 十: 10, 百: 100, 千: 1000 }
  if (!/[十百千]/.test(text))
    return Number(
      text
        .split('')
        .map((c) => digits[c])
        .join(''),
    )
  let total = 0,
    digit = null,
    previous = 10000,
    zero = false
  for (let i = 0; i < text.length; i++) {
    let c = text[i],
      u = units[c]
    if (u) {
      if (u >= previous || zero || (digit === null && !(u === 10 && i === 0)))
        throw Error('中文集号无效')
      total += (digit === null ? 1 : digit) * u
      previous = u
      digit = null
    } else if (digits[c] === 0) {
      if (
        !total ||
        digit !== null ||
        zero ||
        previous === 10 ||
        i === text.length - 1
      )
        throw Error('中文集号无效')
      zero = true
    } else {
      if (digit !== null || digits[c] === undefined) throw Error('中文集号无效')
      digit = digits[c]
      zero = false
    }
  }
  return total + (digit === null ? 0 : digit)
}

function extract(ctx) {
  const out = []
  for (const s of ctx.sources) {
    for (const m of scan(
      ctx,
      s,
      /s(\d+)e(\d+)((?:\s*(?:-\s*e?|e)\d+)*)|\b(\d+)x(\d+)/gi,
    )) {
      let n = Number(m[1] || m[4]),
        episode = m[2] || m[5],
        valid = n <= 999 && episode.length <= 4
      out.push(
        item(
          s,
          m.index,
          m.index + m[0].length,
          n,
          'explicit',
          valid ? '季集标记' : '季集标记中的数字超出范围',
          valid ? 'valid' : 'invalid',
        ),
      )
    }
    for (const m of scan(
      ctx,
      s,
      /(?:^|[ ._\-\[【(])s(\d{1,3})(?=$|[ ._\-\]】)])/gi,
    ))
      out.push(
        item(
          s,
          m.index,
          m.index + m[0].length,
          Number(m[1]),
          'explicit',
          '独立季标记',
        ),
      )
    for (const m of scan(
      ctx,
      s,
      /第\s*([0-9零〇一二两三四五六七八九十百千]+)\s*季|season\s*(\d+)/gi,
    )) {
      let n
      try {
        n = number(m[1] || m[2])
      } catch (e) {
        out.push(
          item(
            s,
            m.index,
            m.index + m[0].length,
            null,
            'explicit',
            e.message,
            'invalid',
          ),
        )
        continue
      }
      out.push(
        item(
          s,
          m.index,
          m.index + m[0].length,
          n,
          'explicit',
          '中文或英文季标记',
          n <= 999 ? 'valid' : 'invalid',
        ),
      )
    }
    if (
      s.kind === 'directories' &&
      /^(?:specials?|特别篇|特別篇)$/i.test(
        words(ctx, s)
          .map((t) => t.text)
          .join(' '),
      )
    )
      out.push(item(s, 0, s.text.length, 0, 'explicit', '特别篇目录'))
  }
  return out
}
