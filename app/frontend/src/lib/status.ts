import type { ServerCard } from './api';

/** ok = normal, warn = yellow, crit = red, off = offline (grey). */
export type Level = 'ok' | 'warn' | 'crit' | 'off';

export interface Tag {
  id: 'reboot' | 'updates' | 'images' | 'failed' | 'offline' | 'clean' | 'agentUpdate' | 'agentOld';
  n?: number;
  tone: 'ok' | 'warn' | 'crit' | 'mute';
}

// The same thresholds as the web panel, so a server looks the same in both.
export function levelOf(c: ServerCard): Level {
  if (!c.online) return 'off';
  const s = c.sample;
  if (!s) return 'ok';
  if (s.cpu > 90 || s.mem_pct > 92 || s.disk_pct > 90) return 'crit';
  if (s.cpu > 75 || s.mem_pct > 85 || s.disk_pct > 80 || s.failed_services > 0 || s.reboot_required || s.updates >= 10) return 'warn';
  return 'ok';
}

/** Colour of a meter: warn from 75, critical from 90. */
export function meter(v: number): 'ok' | 'warn' | 'crit' {
  return v >= 90 ? 'crit' : v >= 75 ? 'warn' : 'ok';
}

/** The flag of the agent of a server: a newer version is out (available) or it is no longer supported (unsupported). */
export type AgentFlag = 'available' | 'unsupported' | null;

export function tagsOf(c: ServerCard, agent: AgentFlag = null): Tag[] {
  const s = c.sample;
  if (!c.online) {
    const tags: Tag[] = [{ id: 'offline', tone: 'mute' }];
    if (s && s.updates > 0) tags.push({ id: 'updates', n: s.updates, tone: 'mute' });
    return tags;
  }
  const tags: Tag[] = [];
  if (s) {
    if (s.reboot_required) tags.push({ id: 'reboot', tone: 'warn' });
    if (s.updates > 0) tags.push({ id: 'updates', n: s.updates, tone: s.updates >= 10 ? 'warn' : 'mute' });
    if ((s.image_updates ?? 0) > 0) tags.push({ id: 'images', n: s.image_updates, tone: (s.image_updates ?? 0) >= 5 ? 'warn' : 'mute' });
    if (s.failed_services > 0) tags.push({ id: 'failed', n: s.failed_services, tone: 'crit' });
  }
  if (agent) tags.push({ id: agent === 'unsupported' ? 'agentOld' : 'agentUpdate', tone: agent === 'unsupported' ? 'crit' : 'warn' });
  if (tags.length === 0) tags.push({ id: 'clean', tone: 'ok' });
  return tags;
}

export interface Totals {
  total: number;
  online: number;
  offline: number;
  problems: number;
  critical: number;
  updates: number;
  rebootPending: number;
  cpuAvg: number | null;
}

export function summarize(cards: ServerCard[]): Totals {
  const t: Totals = { total: cards.length, online: 0, offline: 0, problems: 0, critical: 0, updates: 0, rebootPending: 0, cpuAvg: null };
  let cpu = 0;
  for (const c of cards) {
    const lv = levelOf(c);
    if (c.online) t.online++;
    if (lv !== 'ok') t.problems++;
    if (lv === 'crit') t.critical++;
    const s = c.sample;
    if (!s) continue;
    if (s.updates > 0) t.updates += s.updates;
    if (s.reboot_required) t.rebootPending++;
    if (c.online) cpu += s.cpu;
  }
  t.offline = t.total - t.online;
  t.cpuAvg = t.online > 0 ? cpu / t.online : null;
  return t;
}

/** The name of the system without what does not fit a table cell: "Debian GNU/Linux 12 (bookworm)" -> "Debian 12". */
export function shortOs(os: string): string {
  return os.replace(/\s*\([^)]*\)/g, '').replace(/GNU\/Linux\s*/i, '').replace(/^Microsoft\s+/i, '').replace(/\s+/g, ' ').trim() || os;
}

export const isWindows = (os: string): boolean => os.startsWith('Windows');

const MB = 1024 * 1024;
export function fmtRate(bps: number | undefined): string {
  if (bps === undefined || !Number.isFinite(bps)) return '—';
  return bps < MB ? `${Math.round(bps / 1024)} KB/s` : `${(bps / MB).toFixed(1)} MB/s`;
}

export function fmtUptime(seconds: number, u: { d: string; h: string; min: string }): string {
  const s = Math.max(0, Math.floor(seconds));
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  return d > 0 ? `${d}${u.d} ${h}${u.h}` : h > 0 ? `${h}${u.h} ${m}${u.min}` : `${m}${u.min}`;
}

export function fmtAgo(iso: string | null, now: number, u: { d: string; h: string; min: string }): string {
  if (!iso) return '—';
  const ts = Date.parse(iso);
  if (Number.isNaN(ts)) return '—';
  const min = Math.max(0, Math.floor((now - ts) / 60000));
  if (min < 60) return `${min} ${u.min}`;
  const h = Math.floor(min / 60);
  return h < 48 ? `${h} ${u.h}` : `${Math.floor(h / 24)} ${u.d}`;
}

/** The area and the line of a small chart from values 0-100 (the CPU history). */
export function sparkPath(values: number[], w = 160, h = 26): { line: string; area: string } {
  if (values.length === 0) return { line: '', area: '' };
  const pts = values.length === 1 ? [values[0], values[0]] : values;
  const step = w / (pts.length - 1);
  const line = 'M' + pts.map((v, i) => `${(i * step).toFixed(1)},${(h - (Math.min(100, Math.max(0, v)) / 100) * h * 0.86 - 2).toFixed(1)}`).join(' L');
  return { line, area: `${line} L${w},${h} L0,${h}Z` };
}

export function fmtBytes(n: number | undefined): string {
  if (n === undefined || !Number.isFinite(n)) return '—';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v >= 100 || i === 0 ? Math.round(v) : v.toFixed(1)} ${u[i]}`;
}

/** The count of something with its unit of time "40д 4ч" from seconds. */
export function fmtDuration(seconds: number, u: { d: string; h: string; min: string }): string {
  return fmtUptime(seconds, u);
}

/** The Assistant is offered for Windows computers, and for a Linux computer whose agent says it has a graphical console. */
/** The remote desktop is offered on Windows while the service is not turned off. */
export const rdpOffered = (c: { os: string; sample?: { desktop_off?: boolean } | null }): boolean => isWindows(c.os) && c.sample?.desktop_off !== true;

export function assistantOffered(os: string, sample: { gui?: boolean } | null | undefined): boolean {
  return isWindows(os) || sample?.gui === true;
}
