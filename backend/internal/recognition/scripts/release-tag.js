// 发布标签：文件名最后一个 - 之后的发布组，例如 Film.2020.1080p-GROUP。
// 纯数字、E01、画质、TMDB 标记、含点或空格的后缀都不算发布标签。
const QUALITY = "(?i)\\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit)\\b";

function extract(ctx) {
  const name = ctx.filename;
  const index = name.lastIndexOf("-");
  if (index < 0) {
    return null;
  }
  const tag = name.slice(index + 1).trim();
  if (!tag || /^\d+$/.test(tag) || reSearch("(?i)^e\\d+$", tag)) {
    return null;
  }
  const nearId = name.slice(Math.max(0, index - 5)).toLowerCase().includes("tmdb");
  if (nearId || reSearch(QUALITY, tag) || tag.includes(".") || tag.includes(" ")) {
    trace("后缀是元数据，不作为发布标签", tag);
    return null;
  }
  return {value: tag, matchedText: tag};
}
