// Run with: node --test src/lib/settings/app-tab.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const src = (f) => readFileSync(new URL(f, import.meta.url), 'utf8');

test('the tab "Application" has no explaining paragraphs: the hints are behind "?", the notes and the token are folded', () => {
  const s = src('./AppSection.svelte');
  for (const gone of ['set.upd.hint', 'set.upd.channelHint', 'set.upd.token.hint', 'set.tray.hint']) assert.doesNotMatch(s, new RegExp(gone.replace(/\./g, '\\.')), gone);
  assert.match(s, /data-tip=\{t\('set\.upd\.tipChannel'\)\}/);
  assert.match(s, /data-tip=\{t\('set\.upd\.tipToken'\)\}/);
  assert.match(s, /data-tip=\{t\('set\.tray\.tip'\)\}/);
  assert.match(s, /aria-expanded=\{notesOpen\}/);
  assert.match(s, /aria-expanded=\{tokenOpen\}/);
  assert.match(s, /role="radiogroup"/);
});

test('the locales have the short texts of the tab', () => {
  for (const l of ['ru', 'en']) {
    const j = JSON.parse(src(`../locales/${l}.json`));
    for (const k of ['set.upd.availShort', 'set.upd.latestShort', 'set.upd.whatsNew', 'set.upd.tipChannel', 'set.upd.tipToken', 'set.tray.tip', 'set.upd.token.addLink', 'set.upd.token.setLink']) assert.ok(j[k], `${l} ${k}`);
  }
});
