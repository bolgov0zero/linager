/**
 * A small, safe Markdown renderer for notes. Everything is escaped first and the markup is added afterwards,
 * so the result can go into `{@html}` whatever the text holds. Links are limited to http, https and mailto.
 *
 * Supported: headings (#, ##, ###), paragraphs (a line break stays a line break), **bold**, *italic*, ~~strike~~,
 * `code`, fenced code blocks, links and bare http(s) addresses, bulleted, numbered and task lists (with nesting),
 * quotes (>) and horizontal rules. Tables and images are not supported (they stay as text).
 */

function esc(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

const SAFE_URL = /^(https?:\/\/|mailto:)[^\s<>]*$/i;
const MARK = '\u0000';

/** Bold, italic and strike on an escaped string that holds no code or links any more. */
function emphasis(s: string): string {
  return s
    .replace(/\*\*([^*\n]+?)\*\*/g, '<strong>$1</strong>')
    .replace(/(^|[^\w])__([^_\n]+?)__(?!\w)/g, '$1<strong>$2</strong>')
    .replace(/\*([^*\s][^*\n]*?)\*/g, '<em>$1</em>')
    .replace(/(^|[^\w])_([^_\s][^_\n]*?)_(?!\w)/g, '$1<em>$2</em>')
    .replace(/~~([^~\n]+?)~~/g, '<del>$1</del>');
}

/** One line of inline markup. The input is raw text; the output is safe HTML. */
export function renderInline(text: string): string {
  const keep: string[] = [];
  const stash = (html: string): string => {
    keep.push(html);
    return `${MARK}${keep.length - 1}${MARK}`;
  };
  let s = esc(text.replace(/\u0000/g, ''));
  s = s.replace(/`([^`\n]+)`/g, (_m, c: string) => stash(`<code>${c}</code>`));
  s = s.replace(/\[([^\]\n]+)\]\(([^)\s]+)\)/g, (m, label: string, url: string) =>
    SAFE_URL.test(url) ? stash(`<a href="${url}" target="_blank" rel="noopener noreferrer">${emphasis(label)}</a>`) : m,
  );
  s = s.replace(/(^|[\s(])(https?:\/\/[^\s<]+[^\s<.,;:!?)\]])/gi, (_m, pre: string, url: string) =>
    `${pre}${stash(`<a href="${url}" target="_blank" rel="noopener noreferrer">${url}</a>`)}`,
  );
  s = emphasis(s);
  for (let i = 0; i < 4 && s.includes(MARK); i++) {
    s = s.replace(new RegExp(`${MARK}(\\d+)${MARK}`, 'g'), (_m, n: string) => keep[Number(n)] ?? '');
  }
  return s;
}

interface ListItem {
  indent: number;
  ordered: boolean;
  html: string;
}

const LIST_RE = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/;

function taskPrefix(text: string): string {
  const m = /^\[( |x|X)\]\s+(.*)$/.exec(text);
  if (!m) return renderInline(text);
  return `<span class="task">${m[1] === ' ' ? '☐' : '☑'}</span> ${renderInline(m[2])}`;
}

/** Turns consecutive list lines into nested lists by their indentation. */
function renderList(items: ListItem[]): string {
  let out = '';
  const stack: { indent: number; tag: string }[] = [];
  for (const item of items) {
    const tag = item.ordered ? 'ol' : 'ul';
    while (stack.length > 0 && item.indent < stack[stack.length - 1].indent) {
      const top = stack.pop();
      out += `</li></${top?.tag}>`;
    }
    const top = stack[stack.length - 1];
    if (!top || item.indent > top.indent) {
      out += `<${tag}><li>${item.html}`;
      stack.push({ indent: item.indent, tag });
    } else {
      out += `</li><li>${item.html}`;
    }
  }
  while (stack.length > 0) out += `</li></${stack.pop()?.tag}>`;
  return out;
}

/** Renders a note. The result is safe HTML. */
export function renderMarkdown(src: string): string {
  const lines = src.replace(/\r\n?/g, '\n').split('\n');
  const out: string[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.trim() === '') {
      i++;
      continue;
    }
    // fenced code
    if (/^\s*```/.test(line)) {
      const body: string[] = [];
      i++;
      while (i < lines.length && !/^\s*```/.test(lines[i])) body.push(lines[i++]);
      i++;
      out.push(`<pre><code>${esc(body.join('\n'))}</code></pre>`);
      continue;
    }
    // heading
    const h = /^(#{1,6})\s+(.*?)\s*#*\s*$/.exec(line);
    if (h) {
      const level = Math.min(h[1].length, 3) + 2; // # = h3, ## = h4, ### and deeper = h5
      out.push(`<h${level}>${renderInline(h[2])}</h${level}>`);
      i++;
      continue;
    }
    if (/^\s*([-*_])(\s*\1){2,}\s*$/.test(line)) {
      out.push('<hr>');
      i++;
      continue;
    }
    // quote
    if (/^\s*>/.test(line)) {
      const body: string[] = [];
      while (i < lines.length && /^\s*>/.test(lines[i])) body.push(lines[i++].replace(/^\s*>\s?/, ''));
      out.push(`<blockquote>${body.map(renderInline).join('<br>')}</blockquote>`);
      continue;
    }
    // list
    if (LIST_RE.test(line)) {
      const items: ListItem[] = [];
      while (i < lines.length) {
        const m = LIST_RE.exec(lines[i]);
        if (!m) break;
        items.push({ indent: Math.floor(m[1].replace(/\t/g, '  ').length / 2), ordered: /\d/.test(m[2]), html: taskPrefix(m[3]) });
        i++;
      }
      out.push(renderList(items));
      continue;
    }
    // paragraph: up to the next blank line or the next block
    const para: string[] = [];
    while (
      i < lines.length &&
      lines[i].trim() !== '' &&
      !/^\s*```/.test(lines[i]) &&
      !/^#{1,6}\s/.test(lines[i]) &&
      !/^\s*>/.test(lines[i]) &&
      !LIST_RE.test(lines[i])
    ) {
      para.push(lines[i++]);
    }
    out.push(`<p>${para.map(renderInline).join('<br>')}</p>`);
  }
  return out.join('');
}
