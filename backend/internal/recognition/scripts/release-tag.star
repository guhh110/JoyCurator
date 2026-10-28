def extract(ctx):
    index = ctx.text.rfind("-")
    if index < 0:
        return None
    tag = ctx.text[index+1:].strip()
    if not tag or tag.isdigit() or re_search(r"(?i)^e\d+$", tag):
        return None
    if "tmdb" in ctx.text[max(0, index-5):].lower() or re_search("(?i)\\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit)\\b", tag) or "." in tag or " " in tag:
        trace("后缀是元数据，不作为发布标签", tag)
        return None
    return {"value": tag, "matchedText": tag}
