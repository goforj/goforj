const jsonKeywords = new Set(["true", "false", "null"]);

export const escapeHTML = (value: string) =>
  value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");

const wrapToken = (className: string, value: string) => `<span class="${className}">${escapeHTML(value)}</span>`;

export const maybePrettyJSON = (value: string) => {
  const text = String(value || "").trim();
  if (!text || text === "(empty)") return null;
  if (!(text.startsWith("{") || text.startsWith("["))) return null;
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return null;
  }
};

export const formatJSONDisplay = (value: string) => maybePrettyJSON(value) || value;

export const highlightJSON = (value: string) => {
  const text = formatJSONDisplay(value);
  let out = "";
  let i = 0;
  while (i < text.length) {
    const ch = text[i];
    if (ch === '"') {
      let j = i + 1;
      while (j < text.length) {
        if (text[j] === '"' && text[j - 1] !== "\\") {
          j += 1;
          break;
        }
        j += 1;
      }
      const token = text.slice(i, j);
      let k = j;
      while (k < text.length && /\s/.test(text[k])) k += 1;
      if (text[k] === ":") {
        out += wrapToken("text-sky-300", token);
      } else {
        out += wrapToken("text-emerald-300", token);
      }
      i = j;
      continue;
    }
    if (ch === "-" || /\d/.test(ch)) {
      let j = i + 1;
      while (j < text.length && /[\d.eE+-]/.test(text[j])) j += 1;
      out += wrapToken("text-amber-300", text.slice(i, j));
      i = j;
      continue;
    }
    if (/[A-Za-z]/.test(ch)) {
      let j = i + 1;
      while (j < text.length && /[A-Za-z]/.test(text[j])) j += 1;
      const word = text.slice(i, j);
      if (jsonKeywords.has(word)) {
        out += wrapToken("text-fuchsia-300", word);
      } else {
        out += escapeHTML(word);
      }
      i = j;
      continue;
    }
    out += escapeHTML(ch);
    i += 1;
  }
  return out;
};

export const renderBodyHTML = (value: string) => {
  const text = String(value || "");
  if (text === "(empty)") return escapeHTML(text);
  if (maybePrettyJSON(text)) return highlightJSON(text);
  return escapeHTML(text);
};
