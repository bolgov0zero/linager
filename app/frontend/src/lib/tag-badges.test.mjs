// Run with: node --test src/lib/tag-badges.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { letterOf, splitTags } from './tag-badges.ts';
import { pickerOptions } from './server/tag-picker.ts';

const src = (f) => readFileSync(new URL(f, import.meta.url), 'utf8');
const tag = (id, name) => ({ id, name, color: 'gray' });

test('a tag badge is the first letter in capitals, also for Cyrillic and astral characters', () => {
  assert.equal(letterOf('prod'), 'P');
  assert.equal(letterOf('  бэкап'), 'Б');
  assert.equal(letterOf('🔥hot'), '🔥');
  assert.equal(letterOf(''), '?');
});

test('the tags that do not fit fold into "+N" that counts as a badge', () => {
  const t = ['a', 'b', 'c', 'd', 'e'];
  assert.deepEqual(splitTags(t, 5), { shown: t, rest: [] });
  assert.deepEqual(splitTags(t, 3), { shown: ['a', 'b'], rest: ['c', 'd', 'e'] });
  assert.deepEqual(splitTags(['a', 'b', 'c'], 2), { shown: ['a'], rest: ['b', 'c'] });
  assert.deepEqual(splitTags(['a', 'b'], 2), { shown: ['a', 'b'], rest: [] });
  assert.deepEqual(splitTags(['a', 'b'], 1), { shown: [], rest: ['a', 'b'] });
});

test('the tag field offers the tags that fit, the ones that start with the text first, and "create" for a new name', () => {
  const items = [tag(1, 'backup'), tag(2, 'dev'), tag(3, 'db'), tag(4, 'prod')];
  assert.deepEqual(pickerOptions(items, '').map((o) => o.kind), ['tag', 'tag', 'tag', 'tag']);
  const d = pickerOptions(items, ' D ');
  assert.deepEqual(d.map((o) => (o.kind === 'tag' ? o.tag.name : '+' + o.name)), ['dev', 'db', 'prod', '+D']);
  const b = pickerOptions(items, 'b');
  assert.deepEqual(b.map((o) => (o.kind === 'tag' ? o.tag.name : '+' + o.name)), ['backup', 'db', '+b']);
  // a name that exists (in the list or already on the server) is not offered as new
  assert.equal(pickerOptions(items, 'DEV').some((o) => o.kind === 'new'), false);
  assert.equal(pickerOptions(items, 'web', ['Web']).some((o) => o.kind === 'new'), false);
  assert.deepEqual(pickerOptions(items, 'ops  team').at(-1), { kind: 'new', name: 'ops team' });
  assert.deepEqual(pickerOptions([], ''), []);
});

test('the dashboard shows the badges: cards (3), compact cards (2, and one chip less), rows (a column of 5)', () => {
  assert.match(src('./Card.svelte'), /<TagBadges tags=\{card\.tag_items \?\? \[\]\} max=\{3\}/);
  const mini = src('./Mini.svelte');
  assert.match(mini, /<TagBadges tags=\{card\.tag_items \?\? \[\]\} max=\{2\}/);
  assert.match(mini, /visibleChips\(chipsOf\(card, agent\), \(card\.tag_items\?\.length \?\? 0\) > 0 \? 2 : 3\)/);
  const rows = src('./Rows.svelte');
  assert.match(rows, /<TagBadges tags=\{c\.tag_items \?\? \[\]\} max=\{5\}/);
  assert.match(rows, /rows\.tags/);
});

test('the settings of a server use the own tag list, not the browser one', () => {
  const modal = src('./server/ServerSettingsModal.svelte');
  assert.doesNotMatch(modal, /datalist/);
  assert.match(modal, /<TagPicker /);
  const picker = src('./server/TagPicker.svelte');
  assert.match(picker, /role="combobox"/);
  assert.match(picker, /role="listbox"/);
  assert.match(picker, /ArrowDown/);
  assert.match(picker, /Escape/);
});

test('the locales have the texts of the tag list and of the column', async () => {
  for (const l of ['ru', 'en']) {
    const j = JSON.parse(src(`./locales/${l}.json`));
    for (const k of ['ssm.tags.create', 'ssm.tags.keys', 'rows.tags']) assert.ok(j[k], `${l} ${k}`);
  }
});

test('the entry that opens the snippet settings is set apart by a line and is called "Settings"', () => {
  assert.match(src('./Terminal.svelte'), /t\('term\.snippets\.manage'\), sep: true/);
  assert.match(src('./Menu.svelte'), /<hr class="msep"/);
  assert.equal(JSON.parse(src('./locales/ru.json'))['term.snippets.manage'], 'Настройка');
  assert.equal(JSON.parse(src('./locales/en.json'))['term.snippets.manage'], 'Settings');
});

test('the letter of a tag badge is centred on the ink of the capital (the font has more room below the baseline)', () => {
  assert.match(src('../app.css'), /\.tb-l\{[^}]*padding-top:2px/);
});
