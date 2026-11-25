// Run with: node --test src/lib/settings/updates-layout.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const src = (f) => readFileSync(new URL(f, import.meta.url), 'utf8');

test('the tab "Updates" is the summary card, the tabs and one section at a time', () => {
  const s = src('./UpdatesSection.svelte');
  assert.match(s, /role="tablist"/);
  assert.match(s, /role="tabpanel"/);
  for (const id of ['agents', 'cache', 'history', 'cli']) assert.match(s, new RegExp(`id: '${id}'`));
  assert.doesNotMatch(s, /Fold/); // no folds any more: the tabs replace them
  assert.match(s, /<UpdServerPanel /);
  assert.match(s, /\{#if tab === 'agents'\}/);
  // the check stays in the header of the tab, next to the title
  assert.match(s, /onclick=\{check\}/);
});

test('the summary card shows the version, what is available and the facts; the less important ones fold in a narrow window', () => {
  const s = src('./UpdServerPanel.svelte');
  assert.match(s, /class="panel hero"/);
  assert.match(s, /st\.upd\.srv\.availableShort/);
  assert.match(s, /class="ufact fx-extra"/);
  assert.match(s, /class:open=\{detailsOpen\}/);
  assert.doesNotMatch(s, /oncheck/);
  const css = src('../../app.css');
  assert.match(css, /container-type:inline-size/);
  assert.match(css, /@container \(max-width: 640px\)/);
});

test('the agents, the cache and the history are plain panels (no folds)', () => {
  for (const f of ['./UpdAgentsPanel.svelte', './UpdHistory.svelte']) assert.doesNotMatch(src(f), /Fold/, f);
  assert.match(src('./upd-format.ts'), /export const APPLICABLE/);
});

test('the locales have the texts of the tabs', () => {
  for (const l of ['ru', 'en']) {
    const j = JSON.parse(src(`../locales/${l}.json`));
    for (const k of ['st.upd.srv.availableShort', 'st.upd.details', 'st.upd.tab.cache', 'st.upd.tab.history', 'st.upd.tab.cli']) assert.ok(j[k], `${l} ${k}`);
  }
});
