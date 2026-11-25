// Run with: node --test src/lib/edge-scroll.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { edgeSpeed } from './edge-scroll.ts';

const src = (f) => readFileSync(new URL(f, import.meta.url), 'utf8');

test('the picture stands still in the middle and speeds up toward the edge', () => {
  assert.equal(edgeSpeed(300, 600), 0);
  assert.equal(edgeSpeed(56, 600), 0); // the inner border of the zone
  assert.equal(edgeSpeed(0, 600), -22);
  assert.equal(edgeSpeed(600, 600), 22);
  assert.ok(edgeSpeed(28, 600) < 0 && edgeSpeed(28, 600) > -22);
  assert.ok(edgeSpeed(572, 600) > 0 && edgeSpeed(572, 600) < 22);
  assert.equal(edgeSpeed(-30, 600), -22); // the pointer a little outside is the same as at the edge
  assert.equal(edgeSpeed(700, 600), 22);
  assert.ok(Math.abs(edgeSpeed(10, 600)) > Math.abs(edgeSpeed(40, 600)));
});

test('a box without a middle does not scroll by the edges', () => {
  assert.equal(edgeSpeed(5, 100), 0);
  assert.equal(edgeSpeed(95, 100), 0);
});

test('"100%" scrolls by the edges in the Assistant and in the remote desktop, with no scroll bars', () => {
  const asst = src('./assistant/AssistantSession.svelte');
  assert.match(asst, /attachEdgeScroll\(view,/);
  assert.match(asst, /enabled: \(\) => mode === 'real'/);
  assert.match(asst, /export function fitDelta\(\)/);
  const desk = src('./desktop/DesktopSession.svelte');
  assert.match(desk, /attachEdgeScroll\(host,/);
  assert.match(desk, /scrollbar-width:none/);
  assert.match(src('../app.css'), /\.asst-view\.real\{scrollbar-width:none\}/);
});

test('the menus show the chosen mode by the highlight (no tick), have icons, and the full screen stands last', () => {
  const menu = src('./ContextMenu.svelte');
  assert.match(menu, /class:on=\{it\.selected\}/);
  assert.match(menu, /it\.hint/);
  for (const f of ['./assistant/AssistantPane.svelte', './desktop/DesktopPane.svelte']) {
    const s = src(f);
    assert.doesNotMatch(s, /✓/, f);
    const display = s.slice(s.indexOf('const displayItems'), s.indexOf('const moreItems'));
    assert.match(display, /selected:/, f);
    const keys = [...display.matchAll(/key: '(\w+)'/g)].map((m) => m[1]);
    assert.equal(keys.at(-1), 'full', f); // the full screen is the last entry
    assert.match(display, /icon: 'scaling'/, f);
    assert.match(display, /icon: full\.on \? 'minimize-2' : 'maximize'/, f);
  }
  const asst = src('./assistant/AssistantPane.svelte');
  assert.match(asst, /key: 'fitwin', icon: 'proportions'/);
  assert.match(asst, /native\.fitWindow\(d\.dw, d\.dh\)/);
  assert.match(asst, /key: 'off', icon: 'unplug', label: t\('assist\.disconnect'\), danger: true/);
  assert.match(asst, /hint: 'Ctrl\+Shift\+Esc'/);
});

test('the menu "Session" shows the number, the user without the domain, the address of the client, and the kind as a badge', () => {
  const asst = src('./assistant/AssistantPane.svelte');
  assert.match(asst, /const userOf = /);
  assert.match(asst, /lead: String\(s\.id\)/);
  assert.match(asst, /badge: s\.screen \?/);
  assert.match(asst, /sub: s\.type === 'rdp' && s\.client/);
  const menu = src('./ContextMenu.svelte');
  assert.match(menu, /cbadge t-\{it\.badge\.tone\}/);
});

test('the number of a session in the menu does not take the margin of the global .lead (it stood too high)', () => {
  assert.match(src('./ContextMenu.svelte'), /class="clead"/);
  assert.doesNotMatch(src('./ContextMenu.svelte'), /class="lead"/);
  assert.match(src('../app.css'), /\.ctx \.clead\{[^}]*margin:0/);
});
