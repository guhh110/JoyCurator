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
    s = ctx.sources[0],
    protectedRoles = ['tmdbId', ...technical]
  let marked = false
  for (const m of scan(
    ctx,
    s,
    /s(\d+)e(\d+)((?:\s*(?:-\s*e?|e)\d+)*)|\b(\d+)x(\d+)/gi,
    ['tmdbId', ...technical],
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
    protectedRoles,
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
  const roots = runs(ctx, s)
    .filter((r) => r.parentId === null)
    .map((r) => ({
      ...r,
      words: r.words.filter(
        (t) => !overlaps(ctx, s, t.start, t.end, protectedRoles),
      ),
    }))
    .filter((r) => r.words.length)
  if (roots.length === 1 && ctx.sources.length > 1) {
    const list = roots[0].words,
      plain = list.map((t) => t.text).join(' '),
      bare =
        /^(?:([0-9]{1,4})(?:v\d)?|ep?\s*(\d{1,4})|(\d{1,4})\s*[集话話])$/i.exec(
          plain,
        )
    if (bare && !isYear(Number(plain))) {
      out.push(
        item(
          s,
          list[0].start,
          list[list.length - 1].end,
          [number(bare[1] || bare[2] || bare[3])],
          'contextual',
          '父目录内纯集号',
        ),
      )
      return out
    }
  }
  for (const t of ctx.tokens)
    if (t.sourceId === s.id && t.kind === 'group') {
      const list = words(ctx, s).filter((w) => w.parentId === t.id)
      if (
        list.length === 1 &&
        /^\d{1,4}$/.test(list[0].text) &&
        !isYear(Number(list[0].text)) &&
        !overlaps(ctx, s, t.start, t.end, protectedRoles)
      )
        out.push(
          item(
            s,
            t.start,
            t.end,
            [Number(list[0].text)],
            'heuristic',
            '括号数字待目录对比',
          ),
        )
    }
  const last = roots[roots.length - 1],
    list = last && last.words,
    t = list && list[list.length - 1]
  if (t && /^(?:ep?|e)?\d{1,4}$/i.test(t.text)) {
    const raw = t.text.replace(/^ep?/i, ''),
      n = Number(raw),
      previous = list[list.length - 2]
    if (
      !isYear(n) &&
      previous &&
      /[\s._-]/.test(s.text.slice(previous.end, t.start))
    ) {
      const start = /^ep?$/i.test(previous.text) ? previous.start : t.start
      out.push(item(s, start, t.end, [n], 'contextual', '尾部集号'))
    }
  }
  return out
}
