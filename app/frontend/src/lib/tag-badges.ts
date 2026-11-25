// The tags as small letter badges on the dashboard (cards, compact cards, rows): a tag is its first letter on its colour, and the
// full name slides out on hover over the neighbours. The number of badges per place is limited; the rest fold into "+N".
export interface BadgeTag { name: string; color?: string }

/** The first letter of a tag (a whole character, also outside the Basic Multilingual Plane), in capitals. */
export function letterOf(name: string): string {
  const first = [...name.trim()][0] ?? '?';
  return first.toUpperCase();
}

/**
 * Splits the tags into what a place shows and what folds into "+N". `max` counts the badges including the "+N" one: with more
 * tags than that, `max - 1` letters and the "+N" stay. One badge only ("max" 1) is the "+N" alone.
 */
export function splitTags<T>(tags: T[], max: number): { shown: T[]; rest: T[] } {
  if (tags.length <= max) return { shown: tags, rest: [] };
  const keep = Math.max(0, max - 1);
  return { shown: tags.slice(0, keep), rest: tags.slice(keep) };
}
