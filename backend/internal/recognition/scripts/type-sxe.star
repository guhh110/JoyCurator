def extract(ctx):
    text = re_sub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ")
    match = re_search("(?i)s(\\d{1,3})e(\\d{1,4})((?:\\s*(?:-\\s*e?|e)\\d{1,4})*)", text)
    if not match:
        return None
    if match["end"] < len(text) and text[match["end"]].isdigit():
        fail("季集标记中的数字超出范围")
    return {"value": "tv", "matchedText": match["text"]}
