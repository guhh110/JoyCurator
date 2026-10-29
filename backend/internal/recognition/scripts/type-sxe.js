function extract(ctx) {
  var text = reSub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ");
  var match = reSearch("(?i)s(\\d{1,3})e(\\d{1,4})((?:\\s*(?:-\\s*e?|e)\\d{1,4})*)", text);
  if ((!match)) {
    return null;
  }
  if (((match["end"] < text.length) && /^\d+$/.test(text[match["end"]]))) {
    throw new Error("季集标记中的数字超出范围");
  }
  return {"value": "tv", "matchedText": match["text"]};
}
