# Only this rule extracts the TMDB ID label.
def extract(ctx):
    match = re_search("(?i)tmdb[-= ]?(\\d+)", ctx.text)
    if not match:
        trace("没有显式 TMDB ID")
        return None
    return {"value": int(match["groups"][1]), "matchedText": match["text"], "captures": {"id": match["groups"][1]}}
