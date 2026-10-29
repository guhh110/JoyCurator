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
  if ((reSearch("(?i)^(?:specials?|特别篇|特別篇)$", text.trim()) || reSearch("(?i)^(?:season\\s*(\\d{1,3})|s(\\d{1,3})|第\\s*([0-9零一二两三四五六七八九十]{1,3})\\s*季)$", text.trim()))) {
    trace("跳过季目录或特别篇目录", text);
    return null;
  }
  var title = titleText(text);
  return (title ? {"value": title, "matchedText": ctx.text} : null);
}
