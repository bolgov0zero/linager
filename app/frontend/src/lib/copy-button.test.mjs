// Run with: node --test src/lib/copy-button.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const src = (f) => readFileSync(new URL(f, import.meta.url), 'utf8');

test('the dialog of a reboot has a button that copies the name of the server', () => {
  assert.match(src('./RebootServerModal.svelte'), /<CopyButton text=\{server\.name\} \/>/);
  const b = src('./CopyButton.svelte');
  assert.match(b, /navigator\.clipboard\.writeText\(text\)/);
  assert.match(b, /aria-label=/); // an icon button has a name
});
