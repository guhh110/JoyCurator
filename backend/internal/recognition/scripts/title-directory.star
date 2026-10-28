def find_year(text, minimum, maximum):
    text = re_sub("(?i)tmdb[-= ]?(\\d+)", text, " ")
    for match in re_findall(r"\b\d{4}\b", text):
        number = int(match["text"])
        before = re_sub("[\\[【][^\\]】]*[\\]】]", text[:match["start"]], " ")
        title_prefix = re_sub(r"[^\p{L}\p{N}]", before, "")
        accepted = minimum <= number and number <= maximum and bool(title_prefix)
        trace("年份候选判断", {"number": number, "hasTitleBefore": bool(title_prefix), "accepted": accepted})
        if accepted:
            return match
    return None

def title_text(text):
    text = re_sub("(?i)tmdb[-= ]?(\\d+)", text, " ")
    year = find_year(text, 1900, 2099)
    if year:
        text = text[:year["start"]]
    quality = re_search("(?i)\\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit)\\b", text)
    if quality:
        text = text[:quality["start"]]
    text = re_sub("[\\[【][^\\]】]*[\\]】]", text, " ")
    text = " ".join(text.replace(".", " ").replace("_", " ").split()).strip(" -_[](){}【】")
    trace("清理片名", text)
    return text

def extract(ctx):
    text = re_sub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ")
    if re_search("(?i)^(?:specials?|特别篇|特別篇)$", text.strip()) or re_search("(?i)^(?:season\\s*(\\d{1,3})|s(\\d{1,3})|第\\s*([0-9零一二两三四五六七八九十]{1,3})\\s*季)$", text.strip()):
        trace("跳过季目录或特别篇目录", text)
        return None
    title = title_text(text)
    return {"value": title, "matchedText": ctx.text} if title else None
