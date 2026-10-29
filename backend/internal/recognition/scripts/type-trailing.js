function clean(text) {
  text = reSub("(?i)tmdb[-= ]?(\\d+)", text, " ");
  text = reSub("[\\[【][^\\]】]*[\\]】]", text, " ");
  text = reSub("(?i)\\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit)\\b", text, " ");
  return text.replace(new RegExp("^[ \\._\\-\\t]+|[ \\._\\-\\t]+$", "g"), "");
}
function extract(ctx) {
  var text = clean(ctx.text);
  var match = reSearch("(?i)^(.*?)[\\s._-]+(?:ep?|e)?\\.?\\s*(\\d{1,4})$", text);
  if ((!match)) {
    return null;
  }
  var number = Number(match["groups"][2]);
  if (((number <= 0) || ((1900 <= number) && (number <= 2099)))) {
    trace("尾部数字为年份或无效集数", number);
    return null;
  }
  return {"value": "tv", "matchedText": match["text"]};
}
