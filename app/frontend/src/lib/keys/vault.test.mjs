// Run with: node --test src/lib/keys/vault.test.mjs (Node 22.18 or newer strips the types by itself).
import test from 'node:test';
import assert from 'node:assert/strict';
import { MAX_ENTRIES, VaultError, accountOf, blankEntry, cleanEntry, isForAll, isLost, newSession, openVault, sealVault, sectionsFor } from './vault.ts';

// A cheap cost keeps the tests fast; the real default is checked once below.
const FAST = { m: 8192, t: 1, p: 1 };
const PASS = 'correct horse battery staple';

function entry(over = {}) {
  return { ...blankEntry(), name: 'Office admin', login: 'office.admin', domain: 'CORP', password: 'p@ss', ...over };
}

async function expectCode(promise, code) {
  await assert.rejects(promise, (e) => e instanceof VaultError && e.code === code);
}

test('a vault goes through create, save, open and change', async () => {
  const session = await newSession(PASS, FAST);
  const first = [entry({ note: 'note' }), entry({ name: 'Second', type: 'uac', groups: [{ id: '3', name: 'Office' }], servers: [{ id: 'srv-1', name: 'win-1' }] })];
  const bytes = sealVault(first, session);
  const opened = await openVault(bytes, PASS);
  assert.deepEqual(opened.entries, first);
  // saved again with the key that stayed in memory, then opened once more
  opened.entries.push(entry({ name: 'Third' }));
  const again = await openVault(sealVault(opened.entries, opened.session), PASS);
  assert.equal(again.entries.length, 3);
  assert.equal(again.entries[2].name, 'Third');
});

test('every save has its own nonce, and an empty vault is fine', async () => {
  const session = await newSession(PASS, FAST);
  const a = sealVault([], session);
  const b = sealVault([], session);
  assert.notDeepEqual(a, b);
  assert.deepEqual((await openVault(a, PASS)).entries, []);
});

test('a wrong passphrase is an auth error, not a format error', async () => {
  const bytes = sealVault([entry()], await newSession(PASS, FAST));
  await expectCode(openVault(bytes, PASS + 'x'), 'auth');
  await expectCode(openVault(bytes, ''), 'auth');
});

test('the same word in another Unicode form opens the file', async () => {
  const bytes = sealVault([entry()], await newSession('café pass phrase', FAST));
  const opened = await openVault(bytes, 'café pass phrase');
  assert.equal(opened.entries.length, 1);
});

test('a changed byte anywhere is caught, the header included', async () => {
  const bytes = sealVault([entry()], await newSession(PASS, FAST));
  for (const at of [18, 40, 60, bytes.length - 1]) {
    const bad = bytes.slice();
    bad[at] ^= 1;
    await expectCode(openVault(bad, PASS), 'auth');
  }
});

test('a file that is not a vault, a newer version and absurd costs', async () => {
  await expectCode(openVault(new Uint8Array(10), PASS), 'format');
  await expectCode(openVault(new TextEncoder().encode('x'.repeat(200)), PASS), 'format');
  const bytes = sealVault([], await newSession(PASS, FAST));
  const newer = bytes.slice();
  newer[4] = 2;
  await expectCode(openVault(newer, PASS), 'version');
  const greedy = bytes.slice();
  new DataView(greedy.buffer).setUint32(6, 4 << 20); // 4 GiB of memory asked by the file
  await expectCode(openVault(greedy, PASS), 'format');
  await expectCode(openVault(new Uint8Array((1 << 20) + 1), PASS), 'limit');
});

test('too many entries are refused when saving', async () => {
  const session = await newSession(PASS, FAST);
  const many = Array.from({ length: MAX_ENTRIES + 1 }, () => entry());
  assert.throws(() => sealVault(many, session), (e) => e instanceof VaultError && e.code === 'limit');
});

test('the default cost opens in a reasonable time', async () => {
  const t = Date.now();
  const session = await newSession(PASS);
  assert.deepEqual(session.params, { m: 65536, t: 3, p: 1 });
  await openVault(sealVault([entry()], session), PASS);
  assert.ok(Date.now() - t < 20000);
});

test('cleanEntry keeps good entries and drops bad ones', () => {
  assert.ok(cleanEntry(entry()));
  assert.equal(cleanEntry(null), null);
  assert.equal(cleanEntry({ ...entry(), type: 'ssh' }), null);
  assert.equal(cleanEntry({ ...entry(), login: 5 }), null);
  assert.equal(cleanEntry({ ...entry(), name: 'x'.repeat(81) }), null);
  assert.equal(cleanEntry({ ...entry(), groups: [{ id: '1', name: 'a' }, { id: '1', name: 'a' }] }), null); // the same group twice
  assert.equal(cleanEntry({ ...entry(), servers: 'srv' }), null);
  const old = { ...entry() };
  delete old.groups;
  delete old.servers;
  assert.deepEqual(cleanEntry(old)?.groups, []); // missing lists mean "none"
});

test('accountOf joins the domain and the login', () => {
  assert.equal(accountOf({ domain: 'CORP', login: 'ann' }), 'CORP\\ann');
  assert.equal(accountOf({ domain: '', login: 'ann' }), 'ann');
});

test('the keys of a form: this server, its group, all, the rest', () => {
  const known = { servers: new Set(['s1', 's2', 's3']), groups: new Set(['1', '2']) };
  const target = { serverId: 's1', groupId: 1 };
  const list = [
    entry({ name: 'for-all' }),
    entry({ name: 'here', servers: [{ id: 's1', name: 'one' }] }),
    entry({ name: 'here+group', servers: [{ id: 's1', name: 'one' }], groups: [{ id: '1', name: 'g' }] }),
    entry({ name: 'group', groups: [{ id: '1', name: 'g' }] }),
    entry({ name: 'other server', servers: [{ id: 's2', name: 'two' }] }),
    entry({ name: 'other group', groups: [{ id: '2', name: 'h' }] }),
    entry({ name: 'lost', servers: [{ id: 'gone', name: 'old' }] }),
    entry({ name: 'half lost', servers: [{ id: 'gone', name: 'old' }, { id: 's3', name: 'three' }] }),
    entry({ name: 'uac only', type: 'uac' }),
    entry({ name: 'any type', type: 'any', servers: [{ id: 's1', name: 'one' }] }),
  ];
  const rdp = sectionsFor(list, target, known, 'rdp');
  const names = (a) => a.map((e) => e.name);
  assert.deepEqual(names(rdp.here), ['any type', 'here', 'here+group']);
  assert.deepEqual(names(rdp.group), ['group']);
  assert.deepEqual(names(rdp.all), ['for-all', 'lost']);
  assert.deepEqual(names(rdp.other), ['half lost', 'other group', 'other server']);
  const uac = sectionsFor(list, target, known, 'uac');
  assert.deepEqual(names(uac.all), ['uac only']); // the keys of type RDP are not offered for UAC
  assert.deepEqual(names(uac.here), ['any type']);
  assert.ok(isForAll(list[0]) && !isForAll(list[1]));
  assert.ok(isLost(list[6], known) && !isLost(list[7], known) && !isLost(list[0], known));
  // a server without a group does not match a group-bound key
  assert.deepEqual(names(sectionsFor(list, { serverId: 's9', groupId: null }, known, 'rdp').group), []);
});

test('a key bound to a folder is for the servers in the folders under it', () => {
  const known = { servers: new Set(['s1']), groups: new Set(['1', '2', '3', '9']) };
  const onProd = entry({ id: 'k1', name: 'Prod key', groups: [{ id: '1', name: 'Prod' }] });
  const onTest = entry({ id: 'k2', name: 'Test key', groups: [{ id: '9', name: 'Test' }] });
  const list = [onProd, onTest];
  // the server is in Prod / Web (3): the chain is 3, 2 (Web), 1 (Prod)
  const nested = sectionsFor(list, { serverId: 's1', groupId: 3, groupChain: [3, 2, 1] }, known, 'rdp');
  assert.deepEqual(nested.group.map((e) => e.name), ['Prod key']);
  assert.deepEqual(nested.other.map((e) => e.name), ['Test key']);
  // without a chain only the folder itself counts (the way it was before the folders nested)
  const flat = sectionsFor(list, { serverId: 's1', groupId: 3 }, known, 'rdp');
  assert.deepEqual(flat.group, []);
});
