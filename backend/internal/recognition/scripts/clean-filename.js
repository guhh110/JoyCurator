function extract(ctx) {
  var text = reSub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ");
  text = reSub("[\\[【][^\\]】]*[\\]】]", text, " ");
  text = reSub("(?i)\\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit)\\b", text, " ");
  var cleaned = text.replace(new RegExp("^[ \\._\\-\\t]+|[ \\._\\-\\t]+$", "g"), "");
  trace("文件名清理完成", {"before": ctx.text, "after": cleaned});
  return cleaned;
}
