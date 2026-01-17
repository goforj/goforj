const ansiRegex = /\x1b\[(\d+(?:;\d+)*)m/g;

const colorMap: Record<number, string> = {
  30: "#c7c9d1",
  31: "#f07178",
  32: "#98c379",
  33: "#e5c07b",
  34: "#7aa2f7",
  35: "#c792ea",
  36: "#73d6aa",
  37: "#e5e9f0",
  90: "#8a8f9b",
  91: "#f7768e",
  92: "#9ece6a",
  93: "#e0af68",
  94: "#7aa2f7",
  95: "#bb9af7",
  96: "#7dcfff",
  97: "#ffffff",
};

const escapeHtml = (value: string) =>
  value.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

export const ansiToHtml = (value: string) => {
  let result = "";
  let lastIndex = 0;
  let currentColor = "";

  const applySpan = (chunk: string) => {
    if (!chunk) return;
    const safe = escapeHtml(chunk);
    if (!currentColor) {
      result += safe;
      return;
    }
    result += `<span style="color:${currentColor}">${safe}</span>`;
  };

  for (const match of value.matchAll(ansiRegex)) {
    const index = match.index ?? 0;
    const chunk = value.slice(lastIndex, index);
    applySpan(chunk);
    lastIndex = index + match[0].length;
    const codes = match[1].split(";").map((code) => Number.parseInt(code, 10));
    for (const code of codes) {
      if (code === 0 || code === 39) {
        currentColor = "";
        continue;
      }
      if (colorMap[code]) {
        currentColor = colorMap[code];
      }
    }
  }
  applySpan(value.slice(lastIndex));
  return result;
};
