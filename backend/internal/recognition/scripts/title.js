// 片名：先从文件名提取，文件名只剩集号时改用目录名（近到远，跳过季目录）。
// 清理顺序：去掉 TMDB 标记 → 截到季集标记或尾部集号 → 截到年份、画质 → 去掉方括号与分隔符。
//
// 执行过程只记录影响结果的判断（reSearch / reFindAll）；纯字符串整理用原生
// RegExp，不单独记录，最后由一条「清理片名」汇总前后对比。

// SxxExx、1x03、独立 Sxx 三种季集标记，取最靠前的一个截断。
const EPISODE_MARK = "(?i)s\\d{1,3}e\\d{1,4}|\\b\\d{1,3}x\\d{1,4}|(?:^|[ ._-])s\\d{1,3}(?:$|[ ._-])";
const BARE = "(?i)^(?:\\d{1,4}|ep?\\.?\\s*\\d{1,4}|第\\s*(?:\\d{1,4}|[零〇一二两三四五六七八九十百千]{1,16})\\s*[集话話]|\\d{1,4}\\s*[集话話]|\\d{1,4}v\\d)$";
const TRAILING = "(?i)^(.*?)[\\s._-]+(?:ep?|e)?\\.?\\s*(\\d{1,4})$";
const SEASON_FOLDER = "(?i)^(?:specials?|特别篇|特別篇|season\\s*\\d{1,3}|s\\d{1,3}|第\\s*[0-9零一二两三四五六七八九十]{1,3}\\s*季)$";
const QUALITY_WORDS = "2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h[ .]?264|h[ .]?265|hevc|avc|aac|dts|ddp|hdr10?|dv|10bit|8bit";
const QUALITY = "(?i)\\b(?:" + QUALITY_WORDS + ")\\b";

const ID_NATIVE = /tmdb[-= ]?\d+/gi;
const BRACKETS_NATIVE = /[\[【][^\]】]*[\]】]/g;
const QUALITY_NATIVE = new RegExp("\\b(?:" + QUALITY_WORDS + ")\\b", "gi");

function extract(ctx) {
  const fromFile = fileTitle(ctx.filename, ctx.hasParent);
  if (fromFile) {
    return {value: fromFile, matchedText: ctx.filename};
  }
  for (const dir of ctx.directories) {
    const fromDir = folderTitle(dir);
    if (fromDir) {
      return {value: fromDir, matchedText: dir};
    }
  }
  return null;
}

function fileTitle(filename, hasParent) {
  const original = filename;
  let text = withoutId(filename);
  if (reSearch(BARE, clean(text)) && hasParent && !isYear(clean(text))) {
    trace("文件名只有集号，改用目录名", filename);
    return null;
  }
  const mark = reSearch(EPISODE_MARK, text);
  if (mark) {
    text = text.slice(0, mark.start);
  }
  const tail = reSearch(TRAILING, clean(text));
  if (tail) {
    const number = Number(tail.groups[2]);
    if (number >= 0 && !isYear(String(number))) {
      text = tail.groups[1];
    }
  }
  return titleText(text, original);
}

function folderTitle(dir) {
  const text = withoutId(dir);
  if (reSearch(SEASON_FOLDER, text.trim())) {
    trace("跳过季目录或特别篇目录", dir);
    return null;
  }
  return titleText(text, dir);
}

// text 已去掉 TMDB 标记。
function titleText(text, original) {
  const year = findYear(text);
  if (year) {
    text = text.slice(0, year.start);
  }
  const quality = reSearch(QUALITY, text);
  if (quality) {
    text = text.slice(0, quality.start);
  }
  text = text.replace(BRACKETS_NATIVE, " ")
    .replaceAll(".", " ").replaceAll("_", " ").trim().split(/\s+/).join(" ")
    .replace(/^[ \-_\[\](){}【】]+|[ \-_\[\](){}【】]+$/g, "");
  trace("清理片名", {before: original, after: text});
  return text;
}

// 只截掉前面已有片名文字的年份，开头的数字保留为片名。
function findYear(text) {
  for (const match of reFindAll("\\b\\d{4}\\b", text)) {
    const number = Number(match.text);
    const before = text.slice(0, match.start).replace(BRACKETS_NATIVE, " ");
    // goja 的原生正则不支持 \p{…}，这里用 RE2 辅助函数判断前面是否有片名文字。
    if (number >= 1900 && number <= 2099 && reSearch("[\\p{L}\\p{N}]", before)) {
      return match;
    }
  }
  return null;
}

function isYear(text) {
  return /^\d+$/.test(text) && Number(text) >= 1900 && Number(text) <= 2099;
}

// 判断用的文本：去掉方括号与画质词、首尾分隔符（不记录过程）。
function clean(text) {
  return text.replace(ID_NATIVE, " ").replace(BRACKETS_NATIVE, " ").replace(QUALITY_NATIVE, " ")
    .replace(/^[ ._\-\t]+|[ ._\-\t]+$/g, "");
}

// 每个来源只记录一次 TMDB 标记的去除。
function withoutId(text) {
  // 只有确实去掉了 TMDB 标记才记录，普通路径不留这一步。
  const result = text.replace(ID_NATIVE, " ");
  if (result !== text) {
    trace("去掉 TMDB 标记", {before: text, after: result});
  }
  return result;
}
