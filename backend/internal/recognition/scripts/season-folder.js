function extract(ctx) {
  var text = reSub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ");
  text = text.trim();
  if (reSearch("(?i)^(?:specials?|特别篇|特別篇)$", text)) {
    trace("特别篇目录", text);
    return {"value": 0, "matchedText": ctx.text};
  }
  var match = reSearch("(?i)^(?:season\\s*(\\d{1,3})|s(\\d{1,3})|第\\s*([0-9零一二两三四五六七八九十]{1,3})\\s*季)$", text);
  if ((!match)) {
    return null;
  }
  var numeral = nextValue(match["groups"].slice(1));
  if (/^\d+$/.test(numeral)) {
    var number = Number(numeral);
  }
  else {
    var digits = {"零": 0, "一": 1, "二": 2, "两": 2, "三": 3, "四": 4, "五": 5, "六": 6, "七": 7, "八": 8, "九": 9};
    var parts = numeral.split("十");
    if ((parts.length === 1)) {
      number = (digits[numeral] ?? null);
    }
    else {
      if ((parts.length === 2)) {
        number = (((parts[0] ? (digits[parts[0]] ?? 1) : 1) * 10) + (parts[1] ? (digits[parts[1]] ?? 0) : 0));
      }
      else {
        throw new Error("中文季号无效");
      }
    }
    if ((number === null)) {
      throw new Error("中文季号无效");
    }
  }
  trace("季号转换", {"before": numeral, "after": number});
  return {"value": number, "matchedText": match["text"]};
}
function nextValue(parts) {
  for (const part of parts) {
    if (part) {
      return part;
    }
  }
  throw new Error("目录没有季号");
}
