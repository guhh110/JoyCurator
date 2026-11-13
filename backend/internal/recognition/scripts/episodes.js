const cachedWords = new Map(), cachedRuns = new Map()
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
  if (cachedWords.has(source.id)) return cachedWords.get(source.id)
  const result = ctx.tokens
    .filter(
      (t) =>
        t.sourceId === source.id && (t.kind === 'text' || t.kind === 'number'),
    )
    .sort((a, b) => a.start - b.start)
  cachedWords.set(source.id, result)
  return result
}
function runs(ctx, source) {
  if (cachedRuns.has(source.id)) return cachedRuns.get(source.id)
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
  result.sort((a, b) => a.start - b.start)
  cachedRuns.set(source.id, result)
  return result
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
function isYear(n) {
  return n >= 1900 && n <= 2099
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
  const out = [],
    s = ctx.sources[0]
  let marked = false
  for (const m of scan(
    ctx,
    s,
    /s(\d+)e(\d+)((?:\s*(?:-\s*e?|e)\d+)*)|\b(\d+)x(\d+)/gi,
  )) {
    marked = true
    let values = [Number(m[2] || m[5])],
      reason = '季集标记',
      status = 'valid'
    try {
      if ((m[2] || m[5]).length > 4 || Number(m[1] || m[4]) > 999)
        throw Error('季集标记中的数字超出范围')
      for (const part of (m[3] || '').matchAll(/(-?)\s*e?(\d+)/gi)) {
        const n = Number(part[2]),
          last = values[values.length - 1]
        if (n > 9999) throw Error('季集标记中的数字超出范围')
        if (part[1]) {
          if (n <= last || n - values[0] >= 1000) throw Error('合并集范围无效')
          for (let x = last + 1; x <= n; x++) values.push(x)
        } else values.push(n)
      }
      if (new Set(values).size !== values.length)
        throw Error('集数超出范围或重复')
    } catch (e) {
      reason = e.message
      status = 'invalid'
    }
    out.push(
      item(
        s,
        m.index,
        m.index + m[0].length,
        values,
        'explicit',
        reason,
        status,
      ),
    )
  }

  if (marked) return out
  for (const m of scan(
    ctx,
    s,
    /第\s*([0-9零〇一二两三四五六七八九十百千]{1,16})\s*[集话話]/g,
  )) {
    try {
      out.push(
        item(
          s,
          m.index,
          m.index + m[0].length,
          [number(m[1])],
          'explicit',
          '中文集号',
        ),
      )
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
    }
  }
  if (out.length) return out
  // E/EP 和数字加“集/话”是明确标记；裸数字留给目录裁决。
  for (const run of runs(ctx, s)) {
    const text = s.text.slice(run.start, run.end)
    for (const m of text.matchAll(/(?:^|[ ._-])(ep?[ ._-]*(\d+)|((?:\d)+)[集话話])(?=$|[ ._-])/gi)) {
      const raw = m[2] || m[3],
        start = run.start + m.index + m[0].length - m[1].length,
        end = start + m[1].length
      out.push(item(s, start, end, raw.length <= 4 ? [Number(raw)] : null,
        'explicit', raw.length <= 4 ? '显式集号' : '集数超出范围',
        raw.length <= 4 ? 'valid' : 'invalid'))
    }
  }
  if (out.length) return out
  function guess(start, raw) {
    const n = Number(raw)
    if (raw.length > 4 || isYear(n)) return
    if (out.some((c) => c.spans[0].start === start && c.spans[0].end === start + raw.length)) return
    out.push(item(s, start, start + raw.length, [n], 'heuristic', '数字片段，待目录裁决'))
  }
  for (const t of words(ctx, s)) {
    if (t.kind === 'number') guess(t.start, t.text)
    // 季标记后的紧邻数字是可解释的位置，不扫描其他混合词中的数字。
    for (const m of t.text.matchAll(/第[0-9零〇一二两三四五六七八九十百千]+季(\d+)/g))
      guess(t.start + m.index + m[0].length - m[1].length, m[1])
  }
  return out
}
