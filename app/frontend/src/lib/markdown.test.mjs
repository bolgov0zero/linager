// Run with: node --test src/lib/markdown.test.mjs (Node 22.18 or newer strips the types by itself).
import test from 'node:test';
import assert from 'node:assert/strict';
import { renderMarkdown, renderInline } from './markdown.ts';

test('HTML in a note is escaped, never run', () => {
  const html = renderMarkdown('<script>alert(1)</script>\n<img src=x onerror=alert(1)>\n"quote" & \'single\'');
  assert.ok(!html.includes('<script'), html);
  assert.ok(!html.includes('<img'), html);
  assert.ok(html.includes('&lt;script&gt;'), html);
  assert.ok(html.includes('&amp;'), html);
});

test('only http, https and mailto links are made', () => {
  assert.ok(!renderInline('[x](javascript:alert(1))').includes('<a '));
  assert.ok(!renderInline('[x](data:text/html,<b>)').includes('<a '));
  assert.ok(!renderInline('[x](vbscript:msgbox)').includes('<a '));
  const ok = renderInline('[site](https://example.com/a?b=1&c=2)');
  assert.ok(ok.includes('<a href="https://example.com/a?b=1&amp;c=2" target="_blank" rel="noopener noreferrer">site</a>'), ok);
  assert.ok(renderInline('[mail](mailto:a@b.c)').includes('href="mailto:a@b.c"'));
});

test('a quote cannot leave the attribute of a link', () => {
  const html = renderInline('[x](https://a.b/"onmouseover="alert(1))');
  assert.ok(!/"onmouseover=/.test(html) || html.includes('&quot;onmouseover'), html);
  assert.ok(!html.includes(' onmouseover='), html);
});

test('bare addresses become links, once', () => {
  const html = renderInline('see https://example.com/page, ok');
  assert.equal(html, 'see <a href="https://example.com/page" target="_blank" rel="noopener noreferrer">https://example.com/page</a>, ok');
  // an address inside a markdown link is not linked twice
  const both = renderInline('[https://a.b](https://a.b)');
  assert.equal((both.match(/<a /g) ?? []).length, 1, both);
});

test('inline code protects what is inside it', () => {
  assert.equal(renderInline('`a **b** <i>`'), '<code>a **b** &lt;i&gt;</code>');
  assert.equal(renderInline('**bold** and *it* and ~~gone~~'), '<strong>bold</strong> and <em>it</em> and <del>gone</del>');
  assert.equal(renderInline('snake_case_name stays'), 'snake_case_name stays');
});

test('blocks: headings, lists, quote, code, rule, breaks', () => {
  assert.equal(renderMarkdown('# One\n## Two\n### Three'), '<h3>One</h3><h4>Two</h4><h5>Three</h5>');
  assert.equal(renderMarkdown('- a\n- b'), '<ul><li>a</li><li>b</li></ul>');
  assert.equal(renderMarkdown('1. a\n2. b'), '<ol><li>a</li><li>b</li></ol>');
  assert.equal(renderMarkdown('- a\n  - b\n- c'), '<ul><li>a<ul><li>b</li></ul></li><li>c</li></ul>');
  assert.ok(renderMarkdown('- [ ] todo\n- [x] done').includes('☐') && renderMarkdown('- [x] done').includes('☑'));
  assert.equal(renderMarkdown('> a\n> b'), '<blockquote>a<br>b</blockquote>');
  assert.equal(renderMarkdown('```\nx <b> y\n```'), '<pre><code>x &lt;b&gt; y</code></pre>');
  assert.equal(renderMarkdown('---'), '<hr>');
  assert.equal(renderMarkdown('one\ntwo\n\nthree'), '<p>one<br>two</p><p>three</p>');
});

test('empty and odd input does not throw', () => {
  assert.equal(renderMarkdown(''), '');
  assert.equal(renderMarkdown('\n\n  \n'), '');
  assert.doesNotThrow(() => renderMarkdown('```\nunclosed'));
  assert.doesNotThrow(() => renderMarkdown('[](  ) [x]( ) ** __ ~~ `'));
  assert.doesNotThrow(() => renderMarkdown('\u0000 0 \u00001\u0000 raw marks'));
});
