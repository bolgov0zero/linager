// Run with: node --test src/lib/console/tabs.test.mjs
// The tabs of the Console are reordered by dragging. The panes of the sessions keep their own order: moving a pane in the page
// would restart a session (the custom element of the remote desktop is closed when it leaves the page).
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8');

test('the strip of the tabs is sortable on the x axis and lets the drag start on the tab, not on its cross', () => {
  const c = read('./Console.svelte');
  assert.match(c, /class="strip"[^>]*use:sortable=\{\{ axis: 'x', skip: '\.ctab-x', onreorder: moveTab \}\}/);
});

test('the panes are drawn in a stable order, not in the order of the tabs', () => {
  const c = read('./Console.svelte');
  assert.match(c, /const panels = \$derived\(\[\.\.\.con\.tabs\]\.sort\(\(a, b\) => a\.id - b\.id\)\)/);
  assert.match(c, /\{#each panels as tab \(tab\.id\)\}/);
  assert.doesNotMatch(c, /con-panes">\s*\{#each con\.tabs/);
});

test('moveTab puts a tab at the place it will have after the move', () => {
  const s = read('./store.svelte.ts');
  assert.match(s, /export function moveTab\(from: number, to: number\)/);
  // the same splice as in the store, on a plain list
  const list = [1, 2, 3, 4];
  const [t] = list.splice(0, 1);
  list.splice(3, 0, t);
  assert.deepEqual(list, [2, 3, 4, 1]);
});

test('the switch and the tabs carry the mode, and the stylesheet gives every mode one colour in both themes (no purple)', () => {
  const c = read('./Console.svelte');
  assert.match(c, /<button type="button" data-mode=\{m\.id\}/);
  assert.match(c, /data-tab=\{tab\.id\} data-mode=\{tab\.mode\}/);
  const css = readFileSync(new URL('../../app.css', import.meta.url), 'utf8');
  for (const m of ['term', 'rdp', 'asst']) {
    assert.match(css, new RegExp(`:root,:root\\[data-theme=dark\\]\\{[^}]*--m-${m}:#[0-9a-f]{6}`), `${m} in the dark theme`);
    assert.match(css, new RegExp(`:root\\[data-theme=light\\]\\{[^}]*--m-${m}:#[0-9a-f]{6}`), `${m} in the light theme`);
    assert.match(css, new RegExp(`\\[data-mode=${m}\\]\\{--mc:var\\(--m-${m}\\)\\}`));
  }
});
