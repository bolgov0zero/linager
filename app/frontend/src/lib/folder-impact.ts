// What a change of the folders does to the access of the users, as text for a confirmation. A folder carries access (it covers
// everything under it), so a move or a delete can give or take servers; the server computes the preview, this only words it.
import type { AccessChange } from './api';
import { t } from './i18n.svelte';

/** One line per user: who stops and who starts to see which servers (the server sends the first names and how many in all). */
export function impactLines(impact: readonly AccessChange[]): string[] {
  const names = (list: string[], n: number): string => list.join(', ') + (n > list.length ? t('fld.impact.more', { n: n - list.length }) : '');
  return impact.map((x) => {
    const parts: string[] = [];
    if (x.lose_n > 0) parts.push(t('fld.impact.lose', { names: names(x.lose, x.lose_n) }));
    if (x.gain_n > 0) parts.push(t('fld.impact.gain', { names: names(x.gain, x.gain_n) }));
    return `${x.login}: ${parts.join('; ')}`;
  });
}
