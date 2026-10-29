function extract(ctx) {
  var text = reSub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ");
  var match = reSearch("(?i)s(\\d{1,3})e(\\d{1,4})((?:\\s*(?:-\\s*e?|e)\\d{1,4})*)", text);
  if ((!match)) {
    return null;
  }
  if (((match["end"] < text.length) && /^\d+$/.test(text[match["end"]]))) {
    throw new Error("季集标记中的数字超出范围");
  }
  var first = Number(match["groups"][2]);
  var episodes = [first];
  for (const part of reFindAll("(?i)(-?)\\s*e?(\\d{1,4})", match["groups"][3])) {
    var number = Number(part["groups"][2]);
    if ((part["groups"][1] === "-")) {
      if (((number <= episodes[episodes.length - 1]) || ((number - first) >= 1000))) {
        throw new Error("合并集范围无效");
      }
      episodes.push(...Array.from({length: (number + 1) - (episodes[episodes.length - 1] + 1)}, (_, i) => i + (episodes[episodes.length - 1] + 1)));
    }
    else {
      episodes.push(number);
    }
    trace("展开季集片段", {"fragment": part["text"], "episodes": episodes});
  }
  return {"value": episodes, "matchedText": match["text"]};
}
