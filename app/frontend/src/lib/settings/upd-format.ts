import { ApiError, type UpdateStep } from '../api';
import { i18n, t } from '../i18n.svelte';

export const SERVER_STEPS: UpdateStep['id'][] = ['verify', 'backup', 'stop', 'replace', 'migrate', 'start'];

const bcp47 = (): string => (i18n.locale === 'ru' ? 'ru-RU' : 'en-US');

/** Unix seconds to "сегодня, 09:58" or "14 нояб., 02:00". Zero means never. */
export function fmtDateTime(ts: number): string {
  if (!ts) return '—';
  const date = new Date(ts * 1000);
  const time = date.toLocaleTimeString(bcp47(), { hour: '2-digit', minute: '2-digit' });
  if (date.toDateString() === new Date().toDateString()) return `${t('st.upd.today')}, ${time}`;
  return `${fmtDate(ts)}, ${time}`;
}

/** Unix seconds to "28 окт.". Zero means never. */
export function fmtDate(ts: number): string {
  return ts ? new Date(ts * 1000).toLocaleDateString(bcp47(), { day: 'numeric', month: 'short' }) : '—';
}

/** "0,3 с", "12 с", "1 мин 5 с" for a duration in milliseconds. */
export function fmtDuration(ms: number): string {
  const sec = Math.max(ms, 0) / 1000;
  if (sec < 10) {
    const n = new Intl.NumberFormat(bcp47(), { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(Math.max(sec, 0.1));
    return t('st.upd.dur.s', { n });
  }
  const whole = Math.round(sec);
  if (whole < 60) return t('st.upd.dur.s', { n: whole });
  return t('st.upd.dur.min', { m: Math.floor(whole / 60), s: whole % 60 });
}

export function fmtSize(bytes: number): string {
  if (bytes <= 0) return '—';
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} ${t('st.upd.unit.kb')}`;
  if (bytes < 1024 ** 3) return `${(bytes / 1024 / 1024).toFixed(1)} ${t('st.upd.unit.mb')}`;
  return `${(bytes / 1024 ** 3).toFixed(2)} ${t('st.upd.unit.gb')}`;
}

export function errText(error: unknown): string {
  if (error instanceof ApiError && error.code) return error.code;
  if (error instanceof Error) return error.message;
  return String(error);
}
