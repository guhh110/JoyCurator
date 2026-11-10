// 仅拆分文本，所有位置均为 UTF-16。
function extract(ctx) {
  const han = [
    [0x2e80, 0x2e99],
    [0x2e9b, 0x2ef3],
    [0x2f00, 0x2fd5],
    [0x3005, 0x3005],
    [0x3007, 0x3007],
    [0x3021, 0x3029],
    [0x3038, 0x303b],
    [0x3400, 0x4dbf],
    [0x4e00, 0x9fff],
    [0xf900, 0xfa6d],
    [0xfa70, 0xfad9],
    [0x16fe2, 0x16fe3],
    [0x16ff0, 0x16ff1],
    [0x20000, 0x2a6df],
    [0x2a700, 0x2b739],
    [0x2b740, 0x2b81d],
    [0x2b820, 0x2cea1],
    [0x2ceb0, 0x2ebe0],
    [0x2ebf0, 0x2ee5d],
    [0x2f800, 0x2fa1d],
    [0x30000, 0x3134a],
    [0x31350, 0x323af],
  ]
  const result = [],
    opens = { '[': ']', '【': '】', '(': ')', '（': '）', '{': '}' }
  for (const source of ctx.sources) {
    const text = source.text,
      pairs = new Map(),
      stack = []
    for (let i = 0; i < text.length; i++) {
      const ch = text[i]
      if (opens[ch]) stack.push(i)
      else if (stack.length && opens[text[stack[stack.length - 1]]] === ch)
        pairs.set(stack.pop(), i)
    }
    function isWord(ch) {
      const code = ch.codePointAt(0)
      return (
        /[A-Za-z0-9]/.test(ch) ||
        han.some(([lo, hi]) => code >= lo && code <= hi)
      )
    }
    function add(type, start, end, parentId) {
      const token = {
        id: 't' + result.length,
        sourceId: source.id,
        parentId,
        kind: type,
        text: text.slice(start, end),
        start,
        end,
      }
      result.push(token)
      return token.id
    }
    function walk(start, end, parentId, depth) {
      if (depth > 16) throw new Error('分组深度超过 16 层')
      let i = start
      while (i < end) {
        const close = pairs.get(i)
        if (close !== undefined && close < end) {
          const id = add('group', i, close + 1, parentId)
          walk(i + 1, close, id, depth + 1)
          i = close + 1
          continue
        }
        const ch = String.fromCodePoint(text.codePointAt(i)),
          word = isWord(ch)
        let j = i + ch.length
        while (j < end && !pairs.has(j)) {
          const next = String.fromCodePoint(text.codePointAt(j))
          if (isWord(next) !== word) break
          j += next.length
        }
        const type = word
          ? /^[0-9]+$/.test(text.slice(i, j))
            ? 'number'
            : 'text'
          : 'separator'
        add(type, i, j, parentId)
        i = j
      }
    }
    walk(0, text.length, null, 0)
  }
  trace('分词完成', { sources: ctx.sources.length, tokens: result.length })
  return result
}
