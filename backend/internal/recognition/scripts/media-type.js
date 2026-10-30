// 媒体类型：季号或集数任一已识别即为剧集（tv）。
// 季集规则先于本规则执行，因此这里直接复用它们的结果，不再重复匹配一遍文件名。
// 默认不推断电影（movie）：缺少季集信息只说明“未知”，交给后续 TMDB 匹配决定。
function extract(ctx) {
  const season = ctx.metadata.season;
  const episodes = ctx.metadata.episodes;
  trace("已识别的季集", {season: season, episodes: episodes});
  if (season !== null || episodes.length > 0) {
    return {value: "tv", matchedText: season !== null ? "季号 " + season : "集数 " + episodes.join(",")};
  }
  return null;
}
