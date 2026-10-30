// 季号：按优先级依次尝试，先命中优先。
//   1. 文件名 SxxExx        Show.S02E03.mkv
//   2. 文件名 1x03          Show.1x03.mkv
//   3. 文件名独立 Sxx 标记   Show.S02.mkv
//   4. 季目录（近到远）      Season 2 / S02 / 第二季 / Specials
// 某种写法出错时记录原因并继续尝试下一种；全部未命中且有错误时报告第一个错误。
const SXE = "(?i)s(\\d{1,3})e(\\d{1,4})((?:\\s*(?:-\\s*e?|e)\\d{1,4})*)";
const CROSS = "(?i)\\b(\\d{1,3})x(\\d{1,4})";
const MARKER = "(?i)(?:^|[ ._-])s(\\d{1,3})(?:$|[ ._-])";
const SPECIALS = "(?i)^(?:specials?|特别篇|特別篇)$";
const FOLDER = "(?i)^(?:season\\s*(\\d{1,3})|s(\\d{1,3})|第\\s*([0-9零一二两三四五六七八九十]{1,3})\\s*季)$";

function extract(ctx) {
  const name = withoutId(ctx.filename);
  return firstHit([
    () => seasonOf(marked(SXE, name)),
    () => seasonOf(marked(CROSS, name)),
    () => seasonOf(reSearch(MARKER, name)),
    ...ctx.directories.map((dir) => () => folderSeason(dir)),
  ]);
}

function seasonOf(match) {
  return match ? {value: Number(match.groups[1]), matchedText: match.text} : null;
}

function folderSeason(dir) {
  const text = withoutId(dir).trim();
  if (reSearch(SPECIALS, text)) {
    trace("特别篇目录", dir);
    return {value: 0, matchedText: dir};
  }
  const match = reSearch(FOLDER, text);
  if (!match) {
    return null;
  }
  const numeral = match.groups.slice(1).find((part) => part);
  const number = /^\d+$/.test(numeral) ? Number(numeral) : chineseNumber(numeral);
  trace("季号转换", {before: numeral, after: number});
  return {value: number, matchedText: match.text};
}

function chineseNumber(numeral) {
  const digits = {"零": 0, "一": 1, "二": 2, "两": 2, "三": 3, "四": 4, "五": 5, "六": 6, "七": 7, "八": 8, "九": 9};
  const parts = numeral.split("十");
  let number = null;
  if (parts.length === 1) {
    number = digits[numeral] ?? null;
  } else if (parts.length === 2) {
    number = (parts[0] ? (digits[parts[0]] ?? 1) : 1) * 10 + (parts[1] ? (digits[parts[1]] ?? 0) : 0);
  }
  if (number === null) {
    throw new Error("中文季号无效");
  }
  return number;
}

// 季集标记后紧跟数字说明集号超过四位，不能截断后当作有效值。
function marked(pattern, text) {
  const match = reSearch(pattern, text);
  if (match && match.end < text.length && /^\d$/.test(text[match.end])) {
    throw new Error("季集标记中的数字超出范围");
  }
  return match;
}

function withoutId(text) {
  return reSub("(?i)tmdb[-= ]?(\\d+)", text, " ");
}

function firstHit(attempts) {
  let failure = null;
  for (const attempt of attempts) {
    try {
      const result = attempt();
      if (result) {
        return result;
      }
    } catch (error) {
      trace("此写法出错，继续尝试下一种", String(error.message ?? error));
      failure = failure ?? error;
    }
  }
  if (failure) {
    throw failure;
  }
  return null;
}
