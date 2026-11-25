// Run with: node --test src/lib/keys/state.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { keyStateOf } from './state.ts';

test('open is green, waiting for a save is amber, closed is red only when a vault is known', () => {
  assert.equal(keyStateOf(true, false, true), 'open');
  assert.equal(keyStateOf(true, true, true), 'waiting');
  assert.equal(keyStateOf(false, false, true), 'closed');
  assert.equal(keyStateOf(false, false, false), 'none'); // never opened and nothing remembered: no dot
  assert.equal(keyStateOf(false, true, false), 'none'); // a stale "waiting" without a vault shows nothing
  assert.equal(keyStateOf(true, false, false), 'open'); // opening makes the vault known (the store does that), but open is open anyway
});

test('the topbar button and the button of the panel show the dot and say the state in words', () => {
  const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8');
  for (const file of ['../Shell.svelte', './KeyPicker.svelte']) {
    const s = read(file);
    assert.match(s, /data-keys=\{\$keyState\}/, `${file} has the state on the button`);
    assert.match(s, /keys\.state\./, `${file} says it in the title`);
  }
  const css = read('../../app.css');
  for (const st of ['open', 'waiting', 'closed']) assert.match(css, new RegExp(`\\[data-keys=${st}\\]::after`), `${st} has a colour`);
  for (const lang of ['ru.json', 'en.json']) {
    const d = JSON.parse(read(`../locales/${lang}`));
    for (const k of ['keys.state.open', 'keys.state.waiting', 'keys.state.closed']) assert.ok(d[k], `${lang} ${k}`);
  }
});
