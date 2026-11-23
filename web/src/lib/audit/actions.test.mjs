// Run with: node --test src/lib/audit/actions.test.mjs
// Every action the server writes to the audit log must have a readable text in both languages.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { KNOWN_ACTIONS } from './actions.ts';

const serverDir = new URL('../../../../server/', import.meta.url);
const locale = (lang) => JSON.parse(readFileSync(new URL(`../../locales/${lang}.audit.json`, import.meta.url), 'utf8'));

/** Action names written as string literals next to audit calls; the ones built at run time are listed by hand. */
function serverActions() {
  const found = new Set();
  for (const f of readdirSync(serverDir)) {
    if (!f.endsWith('.go') || f.endsWith('_test.go')) continue;
    const src = readFileSync(new URL(f, serverDir), 'utf8');
    for (const m of src.matchAll(/(?:\.audit\(r, \w+, |Action: |secCall\([^"\n]*"[a-z0-9]+", "[A-Za-z0-9]+", |Audit\([^"\n]*s\.login, )"([a-z0-9_]+(?:\.[a-z0-9_]+)+)"/g)) found.add(m[1]);
  }
  return found;
}

test('every audit action of the server is known and translated', () => {
  const ru = locale('ru');
  const en = locale('en');
  for (const action of serverActions()) {
    assert.ok(KNOWN_ACTIONS.has(action), `not in KNOWN_ACTIONS: ${action}`);
    assert.ok(ru[`audit.action.${action}`], `no Russian text: ${action}`);
    assert.ok(en[`audit.action.${action}`], `no English text: ${action}`);
  }
  for (const action of ['win.users.add', 'win.network.apply', 'service.start', 'docker.container.edit', 'files.copy', 'firewall.draft.reset']) {
    assert.ok(KNOWN_ACTIONS.has(action), `not in KNOWN_ACTIONS: ${action}`);
  }
});

test('every known action has both texts', () => {
  const ru = locale('ru');
  const en = locale('en');
  for (const action of KNOWN_ACTIONS) {
    assert.ok(ru[`audit.action.${action}`], `no Russian text: ${action}`);
    assert.ok(en[`audit.action.${action}`], `no English text: ${action}`);
  }
});
