// Run with: node --test src/lib/desktop/desktop-pane.test.mjs
// The RDP bar has the button "display" (the monitor icon, like the Assistant): the scale, the resolution that follows the window and the
// full screen are in its menu, and the menu "More" keeps the keys and the clipboard.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const src = readFileSync(new URL('./DesktopPane.svelte', import.meta.url), 'utf8');
const block = (name) => {
  const i = src.indexOf(`const ${name} = $derived`);
  assert.ok(i >= 0, `${name} exists`);
  const next = src.indexOf('\n  const ', i + 10);
  const fn = src.indexOf('\n  function ', i + 10);
  return src.slice(i, Math.min(...[next, fn].filter((x) => x > 0)));
};
const keys = (b) => [...b.matchAll(/key: '([a-z]+)'/g)].map((m) => m[1]);

test('the display menu has the scale, the resolution and the full screen', () => {
  assert.deepEqual(keys(block('displayItems')), ['fit', 'real', 'auto', 'full']);
});

test('the More menu no longer has them', () => {
  const more = keys(block('moreItems'));
  for (const k of ['fit', 'real', 'auto', 'full']) assert.ok(!more.includes(k), `${k} is not in More`);
  assert.deepEqual(more, ['tm', 'win', 'lock', 'run', 'alttab', 'send', 'save', 'off']);
});

test('the bar has an icon button with the monitor icon that opens the display menu', () => {
  assert.match(src, /<Icon name="monitor" size=\{14\} \/>/);
  assert.match(src, /open\(displayItems, e\)/);
  assert.match(src, /open\(moreItems, e\)/);
});
