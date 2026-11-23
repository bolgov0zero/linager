// Run with: node --test src/lib/status.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { levelOf, meter, tagsOf, summarize, fmtRate, fmtUptime, sparkPath } from './status.ts';

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
