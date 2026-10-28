def clean(text):
    text = re_sub("(?i)tmdb[-= ]?(\\d+)", text, " ")
    text = re_sub("[\\[【][^\\]】]*[\\]】]", text, " ")
    text = re_sub("(?i)\\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit)\\b", text, " ")
    return text.strip(" ._-\t")

def extract(ctx):
    text = clean(ctx.text)
    match = re_search("(?i)^(?:\\d{1,4}|ep?\\.?\\s*\\d{1,4}|第\\s*\\d{1,4}\\s*[集话話]|\\d{1,4}\\s*[集话話]|\\d{1,4}v\\d)$", text)
    if not match or not ctx.hasParent:
        trace("纯集数需要边界内的父目录", {"hasParent": ctx.hasParent})
        return None
    if text.isdigit() and 1900 <= int(text) and int(text) <= 2099:
        trace("保留数字片名，不作为集数", text)
        return None
    number = re_search(r"\d+", text)["text"]
    return {"value": "tv", "matchedText": text}
