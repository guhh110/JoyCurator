function extract(ctx) {
  var text = reSub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ");
  var match = reSearch("(?i)(?:^|[ ._-])s(\\d{1,3})(?:$|[ ._-])", text);
  if ((!match)) {
    return null;
  }
  return {"value": "tv", "matchedText": match["text"]};
}
