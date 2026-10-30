// 年份：取前面已有片名文字的四位数，因此 1917.mkv、2001.A.Space.Odyssey 里
// 开头的数字会保留为片名。先看文件名，再按近到远查看目录。
// 参数 min / max 限定年份范围，默认 1900–2099。
function extract(ctx) {
  const minimum = ctx.params.min ?? 1900;
  const maximum = ctx.params.max ?? 2099;
  if (minimum < 1 || maximum > 9999 || minimum > maximum) {
    throw new Error("年份参数 min/max 无效");
  }
  for (const text of [ctx.filename, ...ctx.directories]) {
    const match = findYear(text, minimum, maximum);
    if (match) {
      return {value: Number(match.text), matchedText: match.text};
    }
  }
  return null;
}

function findYear(text, minimum, maximum) {
  text = reSub("(?i)tmdb[-= ]?(\\d+)", text, " ");
  for (const match of reFindAll("\\b\\d{4}\\b", text)) {
    const number = Number(match.text);
    const before = reSub("[\\[【][^\\]】]*[\\]】]", text.slice(0, match.start), " ");
    const titlePrefix = reSub("[^\\p{L}\\p{N}]", before, "");
    const accepted = minimum <= number && number <= maximum && Boolean(titlePrefix);
    trace("年份候选判断", {number: number, hasTitleBefore: Boolean(titlePrefix), accepted: accepted});
    if (accepted) {
      return match;
    }
  }
  return null;
}
