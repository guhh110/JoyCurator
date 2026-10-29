function extract(ctx) {
  var index = ctx.text.lastIndexOf("-");
  if ((index < 0)) {
    return null;
  }
  var tag = ctx.text.slice((index + 1)).trim();
  if (((!tag) || /^\d+$/.test(tag) || reSearch("(?i)^e\\d+$", tag))) {
    return null;
  }
  if (((ctx.text.slice(Math.max(0, (index - 5))).toLowerCase().includes("tmdb")) || reSearch("(?i)\\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit)\\b", tag) || (tag.includes(".")) || (tag.includes(" ")))) {
    trace("后缀是元数据，不作为发布标签", tag);
    return null;
  }
  return {"value": tag, "matchedText": tag};
}
