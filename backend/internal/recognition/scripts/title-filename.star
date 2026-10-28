def clean(text):
    text = re_sub("(?i)tmdb[-= ]?(\\d+)", text, " ")
    text = re_sub("[\\[【][^\\]】]*[\\]】]", text, " ")
    text = re_sub("(?i)\\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit)\\b", text, " ")
    return text.strip(" ._-\t")

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
    cleaned = clean(text)
    if re_search("(?i)^(?:\\d{1,4}|ep?\\.?\\s*\\d{1,4}|第\\s*\\d{1,4}\\s*[集话話]|\\d{1,4}\\s*[集话話]|\\d{1,4}v\\d)$", cleaned) and ctx.hasParent and not (cleaned.isdigit() and 1900 <= int(cleaned) and int(cleaned) <= 2099):
        trace("文件名仅有集号，留给目录片名规则")
        return None
    cut = len(text)
    for pattern in ["(?i)s(\\d{1,3})e(\\d{1,4})((?:\\s*(?:-\\s*e?|e)\\d{1,4})*)", "(?i)\\b(\\d{1,3})x(\\d{1,4})", "(?i)(?:^|[ ._-])s(\\d{1,3})(?:$|[ ._-])"]:
        match = re_search(pattern, text)
        if match:
            cut = min(cut, match["start"])
    text = text[:cut]
    tail = re_search("(?i)^(.*?)[\\s._-]+(?:ep?|e)?\\.?\\s*(\\d{1,4})$", clean(text))
    if tail:
        number = int(tail["groups"][2])
        if number > 0 and not (1900 <= number and number <= 2099):
            text = tail["groups"][1]
    title = title_text(text)
    return {"value": title, "matchedText": ctx.text} if title else None
