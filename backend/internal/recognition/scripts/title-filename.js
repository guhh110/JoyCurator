function clean(text) {
  text = reSub("(?i)tmdb[-= ]?(\\d+)", text, " ");
  text = reSub("[\\[【][^\\]】]*[\\]】]", text, " ");
  text = reSub("(?i)\\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit)\\b", text, " ");
  return text.replace(new RegExp("^[ \\._\\-\\t]+|[ \\._\\-\\t]+$", "g"), "");
}
function findYear(text, minimum, maximum) {
  text = reSub("(?i)tmdb[-= ]?(\\d+)", text, " ");
  for (const match of reFindAll("\\b\\d{4}\\b", text)) {
    var number = Number(match["text"]);
    var before = reSub("[\\[【][^\\]】]*[\\]】]", text.slice(0, match["start"]), " ");
    var titlePrefix = reSub("[^\\p{L}\\p{N}]", before, "");
    var accepted = ((minimum <= number) && (number <= maximum) && Boolean(titlePrefix));
    trace("年份候选判断", {"number": number, "hasTitleBefore": Boolean(titlePrefix), "accepted": accepted});
    if (accepted) {
      return match;
    }
  }
  return null;
}
function titleText(text) {
  text = reSub("(?i)tmdb[-= ]?(\\d+)", text, " ");
  var year = findYear(text, 1900, 2099);
  if (year) {
    text = text.slice(0, year["start"]);
  }
  var quality = reSearch("(?i)\\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit)\\b", text);
  if (quality) {
    text = text.slice(0, quality["start"]);
  }
  text = reSub("[\\[【][^\\]】]*[\\]】]", text, " ");
  text = text.replaceAll(".", " ").replaceAll("_", " ").trim().split(/\s+/).join(" ").replace(new RegExp("^[ \\-_\\[\\]\\(\\)\\{\\}【】]+|[ \\-_\\[\\]\\(\\)\\{\\}【】]+$", "g"), "");
  trace("清理片名", text);
  return text;
}
function extract(ctx) {
  var text = reSub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ");
  var cleaned = clean(text);
  if ((reSearch("(?i)^(?:\\d{1,4}|ep?\\.?\\s*\\d{1,4}|第\\s*\\d{1,4}\\s*[集话話]|\\d{1,4}\\s*[集话話]|\\d{1,4}v\\d)$", cleaned) && ctx.hasParent && (!(/^\d+$/.test(cleaned) && (1900 <= Number(cleaned)) && (Number(cleaned) <= 2099))))) {
    trace("文件名仅有集号，留给目录片名规则");
    return null;
  }
  var cut = text.length;
  for (const pattern of ["(?i)s(\\d{1,3})e(\\d{1,4})((?:\\s*(?:-\\s*e?|e)\\d{1,4})*)", "(?i)\\b(\\d{1,3})x(\\d{1,4})", "(?i)(?:^|[ ._-])s(\\d{1,3})(?:$|[ ._-])"]) {
    var match = reSearch(pattern, text);
    if (match) {
      cut = Math.min(cut, match["start"]);
    }
  }
  text = text.slice(0, cut);
  var tail = reSearch("(?i)^(.*?)[\\s._-]+(?:ep?|e)?\\.?\\s*(\\d{1,4})$", clean(text));
  if (tail) {
    var number = Number(tail["groups"][2]);
    if (((number > 0) && (!((1900 <= number) && (number <= 2099))))) {
      text = tail["groups"][1];
    }
  }
  var title = titleText(text);
  return (title ? {"value": title, "matchedText": ctx.text} : null);
}
