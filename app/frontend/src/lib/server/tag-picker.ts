// The options of the tag field in the settings of a server: the tags of the registry that fit what is typed (the ones that start
// with it first), and, when what is typed is not a tag yet, a last option that creates it.
import type { TagItem } from '../api';

export type PickOption = { kind: 'tag'; tag: TagItem } | { kind: 'new'; name: string };

const clean = (s: string): string => s.split(/\s+/).filter(Boolean).join(' ');

/** `items` are the tags that may still be added; `taken` the names already on the server (a name that is taken is not offered as new). */
export function pickerOptions(items: TagItem[], typed: string, taken: string[] = []): PickOption[] {
  const q = clean(typed).toLowerCase();
  const fit = items.filter((x) => !q || x.name.toLowerCase().includes(q));
  const starts = fit.filter((x) => x.name.toLowerCase().startsWith(q));
  const inside = fit.filter((x) => !x.name.toLowerCase().startsWith(q));
  const out: PickOption[] = [...starts, ...inside].map((tag) => ({ kind: 'tag', tag }));
  const known = items.some((x) => x.name.toLowerCase() === q) || taken.some((x) => x.toLowerCase() === q);
  if (q && !known) out.push({ kind: 'new', name: clean(typed) });
  return out;
}
