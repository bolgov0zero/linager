// Run with: node --test src/lib/settings/devices.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { deviceTitle, isMobile, platformKey, versions, waiting } from './devices-util.ts';

const src = (f) => readFileSync(new URL(f, import.meta.url), 'utf8');
const ru = JSON.parse(src('../locales/ru.json'));
const en = JSON.parse(src('../locales/en.json'));

test('a device is called by the name given to it, then by its model, then by its platform', () => {
  assert.equal(deviceTitle({ label: ' Anna iPhone ', model: 'iPhone18,1', platform: 'ios' }, 'iPhone'), 'Anna iPhone');
  assert.equal(deviceTitle({ label: '', model: 'anna-mac', platform: 'macos' }, 'macOS'), 'anna-mac');
  assert.equal(deviceTitle({ label: '', model: '', platform: 'ios' }, 'iPhone'), 'iPhone');
});

test('platforms: an unknown one is "other", phones are told from computers', () => {
  assert.equal(platformKey('ios'), 'st.dev.platform.ios');
  assert.equal(platformKey('plan9'), 'st.dev.platform.other');
  assert.equal(platformKey(''), 'st.dev.platform.other');
  assert.ok(isMobile('ios') && isMobile('android') && !isMobile('macos') && !isMobile(''));
});

test('versions leave out what is not known', () => {
  assert.equal(versions({ app_version: '1.2.3', os_version: '27.1' }), '1.2.3 · 27.1');
  assert.equal(versions({ app_version: '1.2.3', os_version: '' }), '1.2.3');
  assert.equal(versions({ app_version: '', os_version: ' ' }), '');
});

test('the devices that wait are the pending ones', () => {
  const list = [{ id: 1, status: 'approved' }, { id: 2, status: 'pending' }, { id: 3, status: 'denied' }];
  assert.deepEqual(waiting(list).map((d) => d.id), [2]);
});

test('every string the pages of the devices use is in both languages', () => {
  const files = ['./DevicesSection.svelte', '../Waiting.svelte', '../Shell.svelte', '../Login.svelte', '../../App.svelte'];
  const missing = [];
  for (const f of files) {
    for (const m of src(f).matchAll(/\bt\(\s*[`']((?:st\.dev|wait|login\.err\.device)[a-zA-Z0-9_.${}]*)[`']/g)) {
      const key = m[1];
      if (key.includes('${')) continue; // st.dev.status.${d.status}, checked below
      for (const [name, l] of [['ru', ru], ['en', en]]) if (!(key in l)) missing.push(`${name}: ${key}`);
    }
  }
  for (const s of ['pending', 'approved', 'denied']) for (const l of [ru, en]) assert.ok(`st.dev.status.${s}` in l);
  for (const p of ['ios', 'android', 'windows', 'macos', 'linux', 'other']) for (const l of [ru, en]) assert.ok(`st.dev.platform.${p}` in l);
  assert.deepEqual(missing, []);
});

test('the audit knows every action of the devices, in both languages', () => {
  const actions = ['request', 'approve', 'deny', 'remove', 'label', 'gate.on', 'gate.off'];
  const known = src('./audit-actions.ts');
  for (const a of actions) {
    assert.ok(known.includes(`'device.${a}'`), a);
    for (const l of [ru, en]) assert.ok(`st.audit.action.device.${a}` in l, a);
  }
  assert.ok(known.includes("'device.'"));
});

test('the section is for administrators and has its own place in the list', () => {
  const s = src('./sections.ts');
  assert.match(s, /id: 'devices', icon: 'smartphone', admin: true/);
});

test('a device that waits gets the waiting screen, not the application', () => {
  const app = src('../../App.svelte');
  assert.match(app, /stage === 'waiting'/);
  assert.match(app, /device\?\.confirm && s\.device\.status === 'pending'/);
  const w = src('../Waiting.svelte');
  // it asks who it is, and goes on when approved; a refusal or a removal sends it back to the sign-in
  assert.match(w, /api\.me\(\)/);
  assert.match(w, /device_denied/);
  assert.match(w, /e\.status === 401/);
});
