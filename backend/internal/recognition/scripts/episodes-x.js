function extract(ctx) {
  var text = reSub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ");
  var match = reSearch("(?i)\\b(\\d{1,3})x(\\d{1,4})", text);
  if ((!match)) {
    return null;
  }
  if (((match["end"] < text.length) && /^\d+$/.test(text[match["end"]]))) {
    throw new Error("季集标记中的数字超出范围");
  }
  return {"value": Number(match["groups"][2]), "matchedText": match["text"]};
}
