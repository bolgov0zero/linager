// Texts of the Keys that more than one component shows.

import type { Entry, EntryType } from './vault';

type Tr = (key: string, vars?: Record<string, string | number>) => string;

export const TYPES: readonly EntryType[] = ['rdp', 'uac', 'any'];

/** Where a key is used: up to two names as they are, more as counts ("groups: 2, servers: 1"). */
export function bindText(e: Pick<Entry, 'groups' | 'servers'>, tr: Tr): string {
  const g = e.groups.length;
  const s = e.servers.length;
  if (g + s === 0) return tr('keys.bind.all');
  if (g + s <= 2) return [...e.groups.map((x) => tr('keys.bind.group', { name: x.name })), ...e.servers.map((x) => x.name)].join(', ');
  return [g ? tr('keys.bind.g', { n: g }) : '', s ? tr('keys.bind.s', { n: s }) : ''].filter(Boolean).join(', ');
}
