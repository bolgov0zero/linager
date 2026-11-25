// Run with: node --test src/lib/settings/profile-tab.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const src = (f) => readFileSync(new URL(f, import.meta.url), 'utf8');

test('the tab "Profile" is the identity line and three rows that open in place', () => {
  const s = src('./ProfileSection.svelte');
  assert.match(s, /class="pf-who"/);
  assert.equal((s.match(/class="pfr"/g) ?? []).length, 3);
  assert.match(s, /aria-expanded=\{pwOpen\}/);
  assert.match(s, /aria-expanded=\{disOpen\}/);
  assert.match(s, /aria-expanded=\{regenOpen\}/);
  // the three fields of the new password stand in one line
  assert.match(s, /class="pf-fg"/);
  // no big cards and no explaining paragraphs of the old page
  assert.doesNotMatch(s, /class="panel"/);
  for (const gone of ['st.security.totpOn', 'st.security.totpOff', 'st.recovery.hint']) assert.doesNotMatch(s.replace(/title: t\('st\.security\.disable'\)[^\n]*/, ''), new RegExp(gone.replace(/\./g, '\\.')), gone);
});

test('the form of a closed row is not drawn, and a changed password or a disabled 2FA closes its row', () => {
  const s = src('./ProfileSection.svelte');
  assert.match(s, /\{#if pwOpen\}/);
  assert.match(s, /pwOpen = false;/);
  assert.match(s, /disOpen = false;/);
});

test('the locales have the short texts of the tab', () => {
  for (const l of ['ru', 'en']) {
    const j = JSON.parse(src(`../locales/${l}.json`));
    for (const k of ['st.profile.change', 'st.profile.pw.current', 'st.profile.pw.new', 'st.profile.pw.repeat', 'st.profile.pw.submit', 'st.profile.on', 'st.profile.off', 'st.profile.enable', 'st.profile.disable', 'st.recovery.leftShort', 'st.profile.newOnes']) assert.ok(j[k], `${l} ${k}`);
  }
});
