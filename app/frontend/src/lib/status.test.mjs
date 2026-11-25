// Run with: node --test src/lib/status.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { shortOs, levelOf, meter, tagsOf, chipsOf, visibleChips, worstTone, summarize, fmtRate, fmtUptime, fmtBytes, sparkPath } from './status.ts';

const card = (over = {}, sample = {}) => ({
  id: 'a', name: 'a', address: '10.0.0.1', os: 'Debian 12', agent_version: '0.10.0', online: true, last_seen_at: null, spark: [], group_id: null,
  sample: { ts: 1, cpu: 10, mem_pct: 20, disk_pct: 30, rx_bps: 0, tx_bps: 0, uptime: 100, failed_services: 0, reboot_required: false, updates: 0, ...sample },
  ...over,
});

test('the level of a server follows the thresholds of the web panel', () => {
  assert.equal(levelOf(card()), 'ok');
  assert.equal(levelOf(card({ online: false })), 'off');
  assert.equal(levelOf(card({}, { cpu: 91 })), 'crit');
  assert.equal(levelOf(card({}, { disk_pct: 91 })), 'crit');
  assert.equal(levelOf(card({}, { cpu: 76 })), 'warn');
  assert.equal(levelOf(card({}, { failed_services: 1 })), 'warn');
  assert.equal(levelOf(card({}, { reboot_required: true })), 'warn');
  assert.equal(levelOf(card({}, { updates: 10 })), 'warn');
  assert.equal(levelOf(card({ sample: null })), 'ok');
  assert.equal(meter(74), 'ok');
  assert.equal(meter(75), 'warn');
  assert.equal(meter(90), 'crit');
});

test('tags say what needs attention, or that nothing does', () => {
  assert.deepEqual(tagsOf(card()).map((t) => t.id), ['clean']);
  assert.deepEqual(tagsOf(card({}, { reboot_required: true, updates: 12, failed_services: 2 })).map((t) => t.id), ['reboot', 'updates', 'failed']);
  assert.deepEqual(tagsOf(card({ online: false }, { updates: 3 })).map((t) => t.id), ['offline', 'updates']);
});

test('the totals of the dashboard', () => {
  const t = summarize([card({}, { cpu: 20, updates: 3 }), card({ online: false }), card({}, { cpu: 95, reboot_required: true })]);
  assert.deepEqual({ total: t.total, online: t.online, offline: t.offline, problems: t.problems, critical: t.critical, updates: t.updates, reboot: t.rebootPending }, { total: 3, online: 2, offline: 1, problems: 2, critical: 1, updates: 3, reboot: 1 });
  assert.equal(Math.round(t.cpuAvg), 58);
  assert.equal(summarize([]).cpuAvg, null);
});

test('numbers are written the way the panel writes them', () => {
  assert.equal(fmtRate(2048), '2 KB/s');
  assert.equal(fmtRate(3 * 1024 * 1024), '3.0 MB/s');
  assert.equal(fmtRate(undefined), '—');
  const u = { d: 'д', h: 'ч', min: 'м' };
  assert.equal(fmtUptime(90061, u), '1д 1ч');
  assert.equal(fmtUptime(3700, u), '1ч 1м');
  assert.equal(fmtUptime(59, u), '0м');
});

test('a chart needs no data to be drawn without errors', () => {
  assert.deepEqual(sparkPath([]), { line: '', area: '' });
  assert.ok(sparkPath([50]).line.startsWith('M0.0,'));
  assert.ok(sparkPath([10, 90, 50]).area.endsWith('Z'));
});

test('sizes are written the way the panel writes them', () => {
  assert.equal(fmtBytes(0), '0 B');
  assert.equal(fmtBytes(1536), '1.5 KB');
  assert.equal(fmtBytes(5 * 1024 ** 3), '5.0 GB');
  assert.equal(fmtBytes(250 * 1024 ** 2), '250 MB');
  assert.equal(fmtBytes(undefined), '—');
});

test('the name of a system is shortened for a table cell', () => {
  assert.equal(shortOs('Debian GNU/Linux 12 (bookworm)'), 'Debian 12');
  assert.equal(shortOs('Windows 10 Pro (22H2, build 19045)'), 'Windows 10 Pro');
  assert.equal(shortOs('Microsoft Windows Server 2022 Standard'), 'Windows Server 2022 Standard');
  assert.equal(shortOs('Ubuntu 24.04.5 LTS'), 'Ubuntu 24.04.5 LTS');
  assert.equal(shortOs('(x)'), '(x)');
});

test('the chips of the compact card: in the order of their weight, blue for some, amber for many, red for a failure', () => {
  assert.deepEqual(chipsOf(card()), []); // nothing to show: the card shows the address
  const all = chipsOf(card({}, { reboot_required: true, updates: 18, image_updates: 7, failed_services: 2 }), 'available');
  assert.deepEqual(all.map((x) => `${x.id}:${x.tone}${x.n ? ':' + x.n : ''}`), ['failed:crit:2', 'reboot:warn', 'updates:warn:18', 'images:warn:7', 'agentUpdate:warn']);
  // a few packages and a few images are only "there is something" (blue), ten packages and five images are "a lot" (amber)
  assert.deepEqual(chipsOf(card({}, { updates: 7, image_updates: 3 })).map((x) => x.tone), ['info', 'info']);
  assert.deepEqual(chipsOf(card({}, { updates: 10, image_updates: 5 })).map((x) => x.tone), ['warn', 'warn']);
  // Docker updates alone are shown too
  assert.deepEqual(chipsOf(card({}, { image_updates: 3 })).map((x) => x.id), ['images']);
  // an agent that is no longer supported is a failure
  assert.deepEqual(chipsOf(card(), 'unsupported').map((x) => `${x.id}:${x.tone}`), ['agentOld:crit']);
});

test('a server that does not answer shows that, and the updates it knew of, in grey', () => {
  const off = chipsOf(card({ online: false }, { updates: 21, image_updates: 4, reboot_required: true }));
  assert.deepEqual(off.map((x) => `${x.id}:${x.tone}`), ['offline:mute', 'updates:mute']);
  assert.deepEqual(chipsOf(card({ online: false }, { updates: 0 })).map((x) => x.id), ['offline']);
});

test('three chips fit, the rest fold into "+N" with the worst tone', () => {
  const all = chipsOf(card({}, { reboot_required: true, updates: 18, image_updates: 7 }), 'available'); // reboot, updates, images, agent
  const v = visibleChips(all);
  assert.deepEqual(v.shown.map((x) => x.id), ['reboot', 'updates', 'images']);
  assert.deepEqual(v.rest.map((x) => x.id), ['agentUpdate']);
  assert.equal(worstTone(v.rest), 'warn');
  assert.deepEqual(visibleChips(all.slice(0, 3)).rest, []);
  assert.equal(worstTone([{ id: 'updates', tone: 'info' }, { id: 'failed', tone: 'crit' }]), 'crit');
  assert.equal(worstTone([{ id: 'updates', tone: 'info' }]), 'info');
});
