// Run with: node --test src/lib/portal.test.mjs
// The popup of the keys lives outside the floating bar (a transformed ancestor shifts what is `position: fixed` inside it).
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8');

test('the list of the keys is moved out of the bar by the portal action, near its button', () => {
  const s = read('./keys/KeyPicker.svelte');
  assert.match(s, /import \{ portal \} from '\.\.\/portal'/);
  assert.match(s, /<div bind:this=\{list\} class="pop keylist"[^>]*use:portal=\{\{ near: button \}\}/);
});

test('the row of the buttons of the unlock form does not wrap', () => {
  const css = read('../app.css');
  assert.match(css, /\.keyunlock \.krow\{flex-wrap:nowrap\}/);
});

test('the floating bar is the reason: it has a transform or a backdrop filter', () => {
  const css = read('../app.css');
  assert.match(css, /\.fb\{[^}]*backdrop-filter/);
});

test('the unlock popup of the keys is the compact field (variant C): one row, the menu "other file / forget", the arrow', () => {
  const picker = read('./keys/KeyPicker.svelte');
  assert.match(picker, /<KeyOpenForm compact /);
  assert.match(picker, /class:kc-pop=\{!\$vault\}/);
  const form = read('./keys/KeyOpenForm.svelte');
  assert.match(form, /\{#if compact\}/);
  assert.match(form, /placeholder=\{t\('keys\.compact\.ph', \{ name: picked\.name \}\)\}/);
  assert.match(form, /keys\.unlock\.other/);
  assert.match(form, /keys\.file\.forget/);
  // the window "Keys" keeps the full form
  assert.doesNotMatch(read('./keys/KeysModal.svelte'), /KeyOpenForm compact/);
  for (const lang of ['ru.json', 'en.json']) {
    const d = JSON.parse(read(`./locales/${lang}`));
    for (const k of ['keys.compact.ph', 'keys.compact.pick', 'keys.compact.menu']) assert.ok(d[k], `${lang} ${k}`);
  }
});
