import type { F2bSettings } from './api';

export type Preset = 'soft' | 'norm' | 'hard';
export type Unit = 'm' | 'h' | 'd' | 'perm';

/** Ready-made strictness modes: failures before a ban, window in seconds, ban in seconds. */
export const PRESETS: Record<Preset, { maxretry: number; findtime: number; bantime: number }> = {
  soft: { maxretry: 10, findtime: 600, bantime: 600 },
  norm: { maxretry: 5, findtime: 600, bantime: 3600 },
  hard: { maxretry: 3, findtime: 600, bantime: 86400 },
};

export type Translate = (key: string, vars?: Record<string, string | number>) => string;

/** "10 min", "1 h", "2 d" or "forever" for a time in seconds. */
export function durationText(seconds: number, tr: Translate): string {
  if (seconds < 0) return tr('sec.f2b.dur.forever');
  if (seconds % 86400 === 0) return tr('sec.f2b.dur.d', { n: seconds / 86400 });
  if (seconds % 3600 === 0) return tr('sec.f2b.dur.h', { n: seconds / 3600 });
  return tr('sec.f2b.dur.m', { n: Math.max(1, Math.round(seconds / 60)) });
}

/** Splits a ban time into a number and the largest unit it divides by. */
export function splitBan(seconds: number): { value: number; unit: Unit } {
  if (seconds < 0) return { value: 1, unit: 'perm' };
  if (seconds % 86400 === 0) return { value: seconds / 86400, unit: 'd' };
  if (seconds % 3600 === 0) return { value: seconds / 3600, unit: 'h' };
  return { value: Math.max(1, Math.round(seconds / 60)), unit: 'm' };
}

export function joinBan(value: number, unit: Unit): number {
  switch (unit) {
    case 'perm':
      return -1;
    case 'd':
      return value * 86400;
    case 'h':
      return value * 3600;
    default:
      return value * 60;
  }
}

/** Which preset the numbers match, if any. */
export function matchPreset(s: { maxretry: number; findtime: number; bantime: number }): Preset | null {
  for (const [name, p] of Object.entries(PRESETS) as [Preset, (typeof PRESETS)[Preset]][]) {
    if (p.maxretry === s.maxretry && p.findtime === s.findtime && p.bantime === s.bantime) return name;
  }
  return null;
}

/** The text of the settings file the agent writes (a preview for the administrator). */
export function previewConf(s: F2bSettings, self: string): string {
  const ignore = ['127.0.0.1/8', '::1', ...s.ignore.filter((x) => x !== '127.0.0.1/8' && x !== '::1')];
  if (self && !ignore.includes(self)) ignore.push(self);
  const lines = ['[DEFAULT]', `ignoreip = ${ignore.join(' ')}`];
  if (s.increment) lines.push('bantime.increment = true', 'bantime.factor = 2', 'bantime.maxtime = 7d');
  lines.push('', '[sshd]', 'enabled = true', `maxretry = ${s.maxretry}`, `findtime = ${s.findtime}`, `bantime = ${s.bantime}`);
  if (s.recidive) lines.push('', '[recidive]', 'enabled = true', 'bantime = 7d', 'findtime = 1d', 'maxretry = 3');
  return lines.join('\n');
}

export function isIPv4(value: string): boolean {
  const p = value.split('.');
  return p.length === 4 && p.every((x) => /^\d{1,3}$/.test(x) && Number(x) <= 255);
}

export function isIPv6(value: string): boolean {
  return value.includes(':') && /^[0-9a-fA-F:.]+$/.test(value) && value.split(':').length >= 3 && value.split(':').length <= 9;
}
