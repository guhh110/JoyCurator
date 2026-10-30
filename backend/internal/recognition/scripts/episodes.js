// 集数：按优先级依次尝试，先命中优先。
//   1. 文件名 SxxExx       S02E03、S01E01E03、S02E03-E05
//   2. 文件名 1x03         Show.1x03.mkv
//   3. 纯集号文件名        07.mkv、EP04、第03集（需要边界内有父目录）
//   4. 文件名尾部集号      [Group] Show - 01 [1080p]
// 年份（1900–2099）与 TMDB ID 不会被当成集号。
const SXE = "(?i)s(\\d{1,3})e(\\d{1,4})((?:\\s*(?:-\\s*e?|e)\\d{1,4})*)";
const CROSS = "(?i)\\b(\\d{1,3})x(\\d{1,4})";
const BARE = "(?i)^(?:\\d{1,4}|ep?\\.?\\s*\\d{1,4}|第\\s*\\d{1,4}\\s*[集话話]|\\d{1,4}\\s*[集话話]|\\d{1,4}v\\d)$";
const TRAILING = "(?i)^(.*?)[\\s._-]+(?:ep?|e)?\\.?\\s*(\\d{1,4})$";
const QUALITY = "(?i)\\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit)\\b";

function extract(ctx) {
  const name = withoutId(ctx.filename);
  return firstHit([
    () => sxe(name),
    () => cross(name),
    () => bare(clean(ctx.filename), ctx.hasParent),
    () => trailing(clean(ctx.filename)),
  ]);
}

function sxe(text) {
  const match = marked(SXE, text);
  if (!match) {
    return null;
  }
  const first = Number(match.groups[2]);
  const episodes = [first];
  for (const part of reFindAll("(?i)(-?)\\s*e?(\\d{1,4})", match.groups[3])) {
    const number = Number(part.groups[2]);
    if (part.groups[1] === "-") {
      const last = episodes[episodes.length - 1];
      if (number <= last || number - first >= 1000) {
        throw new Error("合并集范围无效");
      }
      for (let episode = last + 1; episode <= number; episode++) {
        episodes.push(episode);
      }
    } else {
      episodes.push(number);
    }
    trace("展开季集片段", {fragment: part.text, episodes: episodes});
  }
  return {value: episodes, matchedText: match.text};
}

function cross(text) {
  const match = marked(CROSS, text);
  return match ? {value: [Number(match.groups[2])], matchedText: match.text} : null;
}

function bare(text, hasParent) {
  if (!reSearch(BARE, text) || !hasParent) {
    trace("纯集号需要边界内的父目录", {hasParent: hasParent});
    return null;
  }
  if (isYear(text)) {
    trace("保留数字片名，不作为集号", text);
    return null;
  }
  return {value: [Number(reSearch("\\d+", text).text)], matchedText: text};
}

function trailing(text) {
  const match = reSearch(TRAILING, text);
  if (!match) {
    return null;
  }
  const number = Number(match.groups[2]);
  if (number <= 0 || isYear(String(number))) {
    trace("尾部数字为年份或无效集号", number);
    return null;
  }
  return {value: [number], matchedText: match.text};
}

function isYear(text) {
  return /^\d+$/.test(text) && Number(text) >= 1900 && Number(text) <= 2099;
}

// 季集标记后紧跟数字说明集号超过四位，不能截断后当作有效值。
function marked(pattern, text) {
  const match = reSearch(pattern, text);
  if (match && match.end < text.length && /^\d$/.test(text[match.end])) {
    throw new Error("季集标记中的数字超出范围");
  }
  return match;
}

function clean(text) {
  text = withoutId(text);
  text = reSub("[\\[【][^\\]】]*[\\]】]", text, " ");
  text = reSub(QUALITY, text, " ");
  return text.replace(/^[ ._\-\t]+|[ ._\-\t]+$/g, "");
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
