def extract(ctx):
    text = re_sub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ")
    text = text.strip()
    if re_search("(?i)^(?:specials?|特别篇|特別篇)$", text):
        trace("特别篇目录", text)
        return {"value": 0, "matchedText": ctx.text}
    match = re_search("(?i)^(?:season\\s*(\\d{1,3})|s(\\d{1,3})|第\\s*([0-9零一二两三四五六七八九十]{1,3})\\s*季)$", text)
    if not match:
        return None
    numeral = next_value(match["groups"][1:])
    if numeral.isdigit():
        number = int(numeral)
    else:
        digits = {"零": 0, "一": 1, "二": 2, "两": 2, "三": 3, "四": 4, "五": 5, "六": 6, "七": 7, "八": 8, "九": 9}
        parts = numeral.split("十")
        if len(parts) == 1:
            number = digits.get(numeral)
        elif len(parts) == 2:
            number = (digits.get(parts[0], 1) if parts[0] else 1) * 10 + (digits.get(parts[1], 0) if parts[1] else 0)
        else:
            fail("中文季号无效")
        if number == None:
            fail("中文季号无效")
    trace("季号转换", {"before": numeral, "after": number})
    return {"value": number, "matchedText": match["text"]}

def next_value(parts):
    for part in parts:
        if part:
            return part
    fail("目录没有季号")
