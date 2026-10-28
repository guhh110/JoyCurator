def extract(ctx):
    text = re_sub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ")
    match = re_search("(?i)(?:^|[ ._-])s(\\d{1,3})(?:$|[ ._-])", text)
    if not match:
        return None
    return {"value": int(match["groups"][1]), "matchedText": match["text"]}
