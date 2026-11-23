export interface Units {
  d: string;
  h: string;
  min: string;
}

export type PluralForm = 'one' | 'few' | 'many';

/** Plural category for a count: Russian uses one/few/many, English one/many. */
export function pluralForm(locale: string, n: number): PluralForm {
  if (locale !== 'ru') return n === 1 ? 'one' : 'many';
  const m10 = n % 10;
  const m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return 'one';
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return 'few';
  return 'many';
}

const MB = 1024 * 1024;

/** Bytes per second as KB/s below 1 MB/s, MB/s above. */
export function fmtRate(bps: number | null | undefined): string {
  if (bps === null || bps === undefined || !Number.isFinite(bps)) return '—';
  if (bps < MB) return `${Math.round(bps / 1024)} KB/s`;
  return `${(bps / MB).toFixed(1)} MB/s`;
}

export function fmtPct(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—';
  return `${Math.round(value)}%`;
}

/** Uptime in seconds as "41д 6ч", "6ч 5м" or "5м". */
export function fmtUptime(seconds: number, u: Units): string {
  const s = Math.max(0, Math.floor(seconds));
  const days = Math.floor(s / 86400);
  const hours = Math.floor((s % 86400) / 3600);
  const minutes = Math.floor((s % 3600) / 60);
  if (days > 0) return `${days}${u.d} ${hours}${u.h}`;
  if (hours > 0) return `${hours}${u.h} ${minutes}${u.min}`;
  return `${minutes}${u.min}`;
}

/** How long ago an ISO timestamp was, as "12 мин", "3 ч" or "2 д"; null when unknown. */
export function fmtAgo(iso: string | null, now: number, u: Units): string | null {
  if (!iso) return null;
  const ts = Date.parse(iso);
  if (Number.isNaN(ts)) return null;
  const minutes = Math.max(0, Math.floor((now - ts) / 60000));
  if (minutes < 60) return `${minutes} ${u.min}`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} ${u.h}`;
  return `${Math.floor(hours / 24)} ${u.d}`;
}
