def extract(ctx):
    text = re_sub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ")
    text = text.strip()
    if re_search("(?i)^(?:specials?|特别篇|特別篇)$", text):
        trace("特别篇目录", text)
        return {"value": "tv", "matchedText": ctx.text}
    match = re_search("(?i)^(?:season\\s*(\\d{1,3})|s(\\d{1,3})|第\\s*([0-9零一二两三四五六七八九十]{1,3})\\s*季)$", text)
    if not match:
        return None
    return {"value": "tv", "matchedText": match["text"]}
