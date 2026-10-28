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

def extract(ctx):
    minimum = ctx.params.get("min", 1900)
    maximum = ctx.params.get("max", 2099)
    if minimum < 1 or maximum > 9999 or minimum > maximum:
        fail("年份参数 min/max 无效")
    match = find_year(ctx.text, minimum, maximum)
    if match:
        return {"value": int(match["text"]), "matchedText": match["text"]}
    return None
