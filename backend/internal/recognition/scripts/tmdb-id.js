// TMDB ID：显式的 tmdb-603、tmdb=603、tmdb 603 标记。
// 先看文件名，再按近到远查看边界内的目录，先命中优先。
const ID = "(?i)tmdb[-= ]?(\\d+)";

function extract(ctx) {
  for (const text of [ctx.filename, ...ctx.directories]) {
    const match = reSearch(ID, text);
    if (match) {
      return {value: Number(match.groups[1]), matchedText: match.text, captures: {id: match.groups[1]}};
    }
  }
  trace("没有显式 TMDB ID");
  return null;
}
