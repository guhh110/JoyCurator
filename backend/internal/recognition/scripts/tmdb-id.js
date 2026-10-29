function extract(ctx) {
  var match = reSearch("(?i)tmdb[-= ]?(\\d+)", ctx.text);
  if ((!match)) {
    trace("没有显式 TMDB ID");
    return null;
  }
  return {"value": Number(match["groups"][1]), "matchedText": match["text"], "captures": {"id": match["groups"][1]}};
}
