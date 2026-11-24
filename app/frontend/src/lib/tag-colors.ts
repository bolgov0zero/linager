// The palette of a tag. The keys are the same as the server accepts (store.TagColors, same order); the shades are drawn by
// the `.tc-<key>` rules of app.css in both themes. There is no purple on purpose: the style of the application has none.
export const TAG_COLORS = [
  'gray', 'slate', 'stone', 'red', 'rose', 'pink', 'orange', 'amber',
  'yellow', 'lime', 'green', 'emerald', 'teal', 'cyan', 'sky', 'blue',
] as const;

export type TagColor = (typeof TAG_COLORS)[number];

/** A key the palette knows, or gray for anything else (a newer server may have more colours). */
export function tagColor(key: string | undefined): TagColor {
  return (TAG_COLORS as readonly string[]).includes(key ?? '') ? (key as TagColor) : 'gray';
}
