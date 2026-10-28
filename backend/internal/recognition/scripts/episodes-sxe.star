def extract(ctx):
    text = re_sub("(?i)tmdb[-= ]?(\\d+)", ctx.text, " ")
    match = re_search("(?i)s(\\d{1,3})e(\\d{1,4})((?:\\s*(?:-\\s*e?|e)\\d{1,4})*)", text)
    if not match:
        return None
    if match["end"] < len(text) and text[match["end"]].isdigit():
        fail("季集标记中的数字超出范围")
    first = int(match["groups"][2])
    episodes = [first]
    for part in re_findall(r"(?i)(-?)\s*e?(\d{1,4})", match["groups"][3]):
        number = int(part["groups"][2])
        if part["groups"][1] == "-":
            if number <= episodes[-1] or number - first >= 1000:
                fail("合并集范围无效")
            episodes.extend(range(episodes[-1] + 1, number + 1))
        else:
            episodes.append(number)
        trace("展开季集片段", {"fragment": part["text"], "episodes": episodes})
    return {"value": episodes, "matchedText": match["text"]}
