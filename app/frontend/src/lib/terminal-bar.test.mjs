// Run with: node --test src/lib/terminal-bar.test.mjs
// The bar of the terminal has the button "in a separate window" like the Assistant and RDP; the Console gives it, the window of the
// terminal itself does not (it is the window already).
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8');

test('the terminal shows the button only when it is given something to call', () => {
  const s = read('./Terminal.svelte');
  assert.match(s, /\{#if onwindow\}<button[^>]*onclick=\{onwindow\}><Icon name="external-link"/);
  assert.match(read('../App.svelte'), /<Terminal id=\{win\.id\}(?![^>]*onwindow)/);
});

test('a Console tab with a terminal moves it to a window of its own', () => {
  const c = read('./console/Console.svelte');
  assert.match(c, /<Terminal [^>]*onwindow=\{\(\) => inWindow\(tab, 'terminal'\)\}/);
  assert.match(c, /kind: 'terminal' \| 'rdp' \| 'asst'/);
});
