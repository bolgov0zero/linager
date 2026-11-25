// Run with: node --test src/lib/display-pref.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { asstDisplayKey, flagValue, parseAsstDisplay, parseFlag, parseRdpScale, rdpAutoKey, rdpScaleKey, SIDEBAR_KEY } from './display-pref.ts';

const src = (f) => readFileSync(new URL(f, import.meta.url), 'utf8');

test('the keys are per server, except the sidebar which is one for all', () => {
  assert.notEqual(asstDisplayKey('a'), asstDisplayKey('b'));
  assert.notEqual(rdpScaleKey('a'), rdpScaleKey('b'));
  assert.notEqual(rdpAutoKey('a'), rdpAutoKey('b'));
  assert.ok(!SIDEBAR_KEY.includes('server') && !/[0-9a-f]{8}-/.test(SIDEBAR_KEY));
});

test('what is read back is checked: an unknown or empty value is the default', () => {
  assert.equal(parseAsstDisplay('real'), 'real');
  assert.equal(parseAsstDisplay('fill'), 'fill');
  assert.equal(parseAsstDisplay('fit'), 'fit');
  assert.equal(parseAsstDisplay('nonsense'), 'fit');
  assert.equal(parseAsstDisplay(null), 'fit');
  assert.equal(parseRdpScale('real'), 'real');
  assert.equal(parseRdpScale('fill'), 'fit'); // the remote desktop has no "fill"
  assert.equal(parseFlag('1', false), true);
  assert.equal(parseFlag('0', true), false);
  assert.equal(parseFlag(null, true), true);
  assert.equal(parseFlag('x', false), false);
  assert.equal(parseFlag(flagValue(true), false), true);
  assert.equal(parseFlag(flagValue(false), true), false);
});

test('the Assistant, the remote desktop and the page of a server keep their choices in the preferences of the application', () => {
  const asst = src('./assistant/AssistantPane.svelte');
  assert.match(asst, /setPref\(asstDisplayKey\(serverId\), v\)/);
  assert.match(asst, /parseAsstDisplay\(getPref\(asstDisplayKey\(serverId\)\)\)/);
  assert.doesNotMatch(asst, /onselect: \(\) => \(display = /); // a choice goes through setDisplay, which saves it
  const desk = src('./desktop/DesktopPane.svelte');
  assert.match(desk, /setPref\(rdpScaleKey\(serverId\), v\)/);
  assert.match(desk, /setPref\(rdpAutoKey\(serverId\), flagValue\(autoResize\)\)/);
  assert.match(desk, /if \(phase === 'open'\) session\?\.setScale\(scale\)/); // a new connection gets the remembered scale
  const page = src('./server/ServerPage.svelte');
  assert.match(page, /setPref\(SIDEBAR_KEY, flagValue\(collapsed\)\)/);
  assert.match(page, /parseFlag\(getPref\(SIDEBAR_KEY\), false\)/);
  // nothing of this goes to the server
  for (const s of [asst, desk]) assert.doesNotMatch(s, /api\.[a-zA-Z]*(display|scale)/i);
});

test('the sidebars of the settings and of "Add server", and the list of the folders, have icons; the chosen option is highlighted, not ticked', () => {
  const sections = src('./settings/sections.ts');
  for (const id of ['app', 'profile', 'users', 'snippets', 'folders', 'tags', 'access', 'connection', 'updates', 'audit']) assert.match(sections, new RegExp(`id: '${id}', icon: '[a-z-]+'`), id);
  assert.match(src('./win/SettingsWindow.svelte'), /<Icon name=\{s\.icon\} size=\{15\} \/>/);
  assert.match(src('./addserver/AddServerModal.svelte'), /TAB_ICON\[id\]/);
  const sel = src('./CustomSelect.svelte');
  assert.match(sel, /\{#if o\.i\}<Icon name=\{o\.i\}/);
  assert.doesNotMatch(sel, /name="check"/);
});
