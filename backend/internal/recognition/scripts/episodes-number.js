function clean(text) {
  text = reSub("(?i)tmdb[-= ]?(\\d+)", text, " ");
  text = reSub("[\\[【][^\\]】]*[\\]】]", text, " ");
  text = reSub("(?i)\\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit)\\b", text, " ");
  return text.replace(new RegExp("^[ \\._\\-\\t]+|[ \\._\\-\\t]+$", "g"), "");
}
function extract(ctx) {
  var text = clean(ctx.text);
  var match = reSearch("(?i)^(?:\\d{1,4}|ep?\\.?\\s*\\d{1,4}|第\\s*\\d{1,4}\\s*[集话話]|\\d{1,4}\\s*[集话話]|\\d{1,4}v\\d)$", text);
  if (((!match) || (!ctx.hasParent))) {
    trace("纯集数需要边界内的父目录", {"hasParent": ctx.hasParent});
    return null;
  }
  if ((/^\d+$/.test(text) && (1900 <= Number(text)) && (Number(text) <= 2099))) {
    trace("保留数字片名，不作为集数", text);
    return null;
  }
  var number = reSearch("\\d+", text)["text"];
  return {"value": Number(number), "matchedText": text};
}
