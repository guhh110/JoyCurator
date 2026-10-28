def clean(text):
    text = re_sub("(?i)tmdb[-= ]?(\\d+)", text, " ")
    text = re_sub("[\\[【][^\\]】]*[\\]】]", text, " ")
    text = re_sub("(?i)\\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit)\\b", text, " ")
    return text.strip(" ._-\t")

def extract(ctx):
    text = clean(ctx.text)
    match = re_search("(?i)^(.*?)[\\s._-]+(?:ep?|e)?\\.?\\s*(\\d{1,4})$", text)
    if not match:
        return None
    number = int(match["groups"][2])
    if number <= 0 or 1900 <= number and number <= 2099:
        trace("尾部数字为年份或无效集数", number)
        return None
    return {"value": number, "matchedText": match["text"]}
