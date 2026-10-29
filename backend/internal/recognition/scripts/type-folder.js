function extract(ctx) {
  var text = reSub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ");
  text = text.trim();
  if (reSearch("(?i)^(?:specials?|特别篇|特別篇)$", text)) {
    trace("特别篇目录", text);
    return {"value": "tv", "matchedText": ctx.text};
  }
  var match = reSearch("(?i)^(?:season\\s*(\\d{1,3})|s(\\d{1,3})|第\\s*([0-9零一二两三四五六七八九十]{1,3})\\s*季)$", text);
  if ((!match)) {
    return null;
  }
  return {"value": "tv", "matchedText": match["text"]};
}
