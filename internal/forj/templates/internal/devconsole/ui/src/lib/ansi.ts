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

const backgroundMap: Record<number, string> = {
  40: "#222430",
  41: "#9d3e3b",
  42: "#4f7a2a",
  43: "#b06f18",
  44: "#1f5ba7",
  45: "#6c3ea5",
  46: "#159c8f",
  47: "#cbd3d8",
  100: "#2e3440",
  101: "#f07178",
  102: "#98c379",
  103: "#e5c07b",
  104: "#7aa2f7",
  105: "#c792ea",
  106: "#73d6aa",
  107: "#f0f0f0",
};

const escapeHtml = (value: string) =>
  value.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

const toAnsi256 = (code: number) => {
  if (code < 0 || code > 255) {
    return "";
  }
  if (code < 16) {
    return colorMap[30 + (code % 8)] ?? "#ffffff";
  }
  if (code >= 16 && code <= 231) {
    const idx = code - 16;
    const r = Math.floor(idx / 36);
    const g = Math.floor((idx % 36) / 6);
    const b = idx % 6;
    const convert = (value: number) => (value === 0 ? 0 : 95 + (value - 1) * 40);
    const toHex = (value: number) => value.toString(16).padStart(2, "0");
    return `#${toHex(convert(r))}${toHex(convert(g))}${toHex(convert(b))}`;
  }
  const gray = 8 + (code-232)*10;
  const toHex = (value: number) => value.toString(16).padStart(2, "0");
  return `#${toHex(gray)}${toHex(gray)}${toHex(gray)}`;
};

export const ansiToHtml = (value: string) => {
  let result = "";
  let lastIndex = 0;
  let currentColor = "";
  let currentBackground = "";

  const applySpan = (chunk: string) => {
    if (!chunk) return;
    const safe = escapeHtml(chunk);
    const styles: string[] = [];
    if (currentColor) {
      styles.push(`color:${currentColor}`);
    }
    if (currentBackground) {
      styles.push(`background:${currentBackground}`);
    }
    if (!styles.length) {
      result += safe;
      return;
    }
    result += `<span style="${styles.join(";")}">${safe}</span>`;
  };

  for (const match of value.matchAll(ansiRegex)) {
    const index = match.index ?? 0;
    const chunk = value.slice(lastIndex, index);
    applySpan(chunk);
    lastIndex = index + match[0].length;
    const codes = match[1].split(";").map((code) => Number.parseInt(code, 10));
    for (let i = 0; i < codes.length; i++) {
      const code = codes[i];
      if (code === 0) {
        currentColor = "";
        currentBackground = "";
        continue;
      }
      if (code === 39) {
        currentColor = "";
        continue;
      }
      if (code === 49) {
        currentBackground = "";
        continue;
      }
      if (code === 38 && codes[i + 1] === 5) {
        const palette = codes[i + 2];
        const color = toAnsi256(palette);
        if (color) {
          currentColor = color;
        }
        i += 2;
        continue;
      }
      if (code === 48 && codes[i + 1] === 5) {
        const palette = codes[i + 2];
        const color = toAnsi256(palette);
        if (color) {
          currentBackground = color;
        }
        i += 2;
        continue;
      }
      if (colorMap[code]) {
        currentColor = colorMap[code];
        continue;
      }
      if (backgroundMap[code]) {
        currentBackground = backgroundMap[code];
        continue;
      }
    }
  }
  applySpan(value.slice(lastIndex));
  return result;
};
