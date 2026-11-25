// Run with: node --test src/lib/keys/autosave.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const src = (f) => readFileSync(new URL(f, import.meta.url), 'utf8');

test('every change of the entries is written into the file by itself', () => {
  const s = src('./store.ts');
  assert.equal((s.match(/scheduleSave\(\);/g) ?? []).length, 2); // putEntry and removeEntry
  assert.match(s, /writeVaultFile\(bytes, state\.sum\)/);
  assert.match(s, /const SAVE_DELAY_MS = 500/);
  // a file changed on the disk is not overwritten without asking
  assert.match(s, /r\.reason === 'changed'/);
  assert.match(s, /writeVaultFile\(bytes, state\.sum, true\)/);
  // a change that comes during a write is written next, and a failed write does not loop
  assert.match(s, /dirty: now\.entries !== state\.entries/);
  assert.match(s, /if \(get\(vault\)\?\.saveError \|\| !get\(autosave\)\) return false;/);
  // the lock writes first
  assert.match(s, /await flush\(\); \/\/ what is not in the file yet is written first/);
});

test('there is no "Save" button while the autosave works; it shows after a failure or where there is no autosave', () => {
  const m = src('./KeysModal.svelte');
  assert.match(m, /\{#if \$vault\.saveError \|\| !\$autosave\}<button/);
  assert.match(m, /keys\.saving/);
  assert.match(m, /if \(\$vault\?\.dirty\) await flush\(\);/);
});

test('the file is written through the application, never to a path of the page', () => {
  const f = src('./vaultFile.ts');
  assert.match(f, /native\.vaultWrite\(toBase64\(bytes\), base, force\)/);
  assert.match(src('../native.ts'), /'PUT', 'vault\/write'/);
});

test('the locales have the texts of the autosave', () => {
  for (const l of ['ru', 'en']) {
    const j = JSON.parse(src(`../locales/${l}.json`));
    for (const k of ['keys.saving', 'keys.saveFailed', 'keys.conflict.title', 'keys.conflict.text', 'keys.conflict.overwrite', 'keys.conflict.other']) assert.ok(j[k], `${l} ${k}`);
  }
});
