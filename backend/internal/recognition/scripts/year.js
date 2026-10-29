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
function extract(ctx) {
  var minimum = (ctx.params["min"] ?? 1900);
  var maximum = (ctx.params["max"] ?? 2099);
  if (((minimum < 1) || (maximum > 9999) || (minimum > maximum))) {
    throw new Error("年份参数 min/max 无效");
  }
  var match = findYear(ctx.text, minimum, maximum);
  if (match) {
    return {"value": Number(match["text"]), "matchedText": match["text"]};
  }
  return null;
}
