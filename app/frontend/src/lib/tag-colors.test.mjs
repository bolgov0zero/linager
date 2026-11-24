// Run with: node --test src/lib/tag-colors.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { TAG_COLORS, tagColor } from './tag-colors.ts';

const read = (rel) => readFileSync(new URL(rel, import.meta.url), 'utf8');

test('the palette is the same as the server accepts, in the same order', () => {
  const go = read('../../../../server/store/tags.go');
  const body = go.match(/var TagColors = \[\]string\{([\s\S]*?)\n\}/)[1];
  const keys = [...body.matchAll(/"([a-z]+)"/g)].map((m) => m[1]);
  assert.deepEqual([...TAG_COLORS], keys);
});

test('every colour is drawn by the stylesheet and has a name in both languages', () => {
  const css = read('../app.css');
  const ru = JSON.parse(read('./locales/ru.json'));
  const en = JSON.parse(read('./locales/en.json'));
  for (const k of TAG_COLORS) {
    assert.match(css, new RegExp(`\\.tc-${k}\\{--tg:#[0-9a-f]{6}\\}`), `the colour ${k} has no CSS rule`);
    assert.ok(ru[`st.tags.c.${k}`], `ru name of ${k}`);
    assert.ok(en[`st.tags.c.${k}`], `en name of ${k}`);
  }
});

test('there is no purple in the palette and an unknown key is drawn gray', () => {
  for (const bad of ['purple', 'violet', 'indigo', 'fuchsia']) assert.ok(!TAG_COLORS.includes(bad));
  assert.equal(tagColor('blue'), 'blue');
  assert.equal(tagColor('magenta'), 'gray');
  assert.equal(tagColor(undefined), 'gray');
});
