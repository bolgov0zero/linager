import { ApiError } from './api';
import { t } from './i18n.svelte';

/** Human-readable text for a failed call of the server: its own message when it sent one. */
export function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 0) return t('common.networkError');
    if (e.status === 503) return t('svc.offline');
    if (e.status === 501) return t('svc.disabled');
    if (e.code) return e.code;
  }
  return t('common.genericError');
}

/** The session of the server has ended (the person signed out, or seven days passed). */
export const isUnauthorized = (e: unknown): boolean => e instanceof ApiError && e.status === 401;
