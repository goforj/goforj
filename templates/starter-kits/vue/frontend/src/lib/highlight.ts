// A small highlighter for the Vue single-file components shown on the
// component reference pages.
//
// It is deliberately not a general purpose parser. The input is the kit's own
// example files, so it only needs to recognise tags, attributes, strings,
// comments, and a handful of keywords. Swap it for Shiki or Highlight.js if
// you start rendering arbitrary source and want full grammar coverage.

const KEYWORDS = new Set([
  'import', 'from', 'export', 'default', 'const', 'let', 'function', 'return',
  'async', 'await', 'if', 'else', 'for', 'of', 'in', 'new', 'type', 'interface',
  'satisfies', 'as', 'true', 'false', 'null', 'undefined',
])

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

function span(className: string, text: string): string {
  return `<span class="${className}">${escapeHtml(text)}</span>`
}

/**
 * Returns HTML with token spans. The caller is responsible for rendering it
 * inside a <pre>; the input is the kit's own source, never user content.
 */
export function highlightVue(source: string): string {
  // Order matters: comments and strings first so their contents are not
  // tokenised again as markup.
  const pattern = new RegExp(
    [
      '(<!--[\\s\\S]*?-->|/\\*[\\s\\S]*?\\*/|//[^\\n]*)', // 1 comment
      '(\'[^\']*\'|"[^"]*"|`[^`]*`)', // 2 string
      '(</?[A-Za-z][\\w.-]*)', // 3 tag open or close
      '(/?>)', // 4 tag end
      '([:@#]?[A-Za-z_][\\w.-]*)(?==)', // 5 attribute name
      '\\b(' + [...KEYWORDS].join('|') + ')\\b', // 6 keyword
    ].join('|'),
    'g',
  )

  let out = ''
  let last = 0

  for (const match of source.matchAll(pattern)) {
    const index = match.index ?? 0
    out += escapeHtml(source.slice(last, index))

    const [text, comment, string, tag, tagEnd, attr, keyword] = match
    if (comment) out += span('text-code-comment', comment)
    else if (string) out += span('text-code-string', string)
    else if (tag) out += span('text-code-tag', tag)
    else if (tagEnd) out += span('text-code-tag', tagEnd)
    else if (attr) out += span('text-code-attr', attr)
    else if (keyword) out += span('text-code-keyword', keyword)
    else out += escapeHtml(text)

    last = index + text.length
  }

  return out + escapeHtml(source.slice(last))
}
