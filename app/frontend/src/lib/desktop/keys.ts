/** What the keyboard layer of the remote desktop does with a key event. */
export type KeyPlan = 'pass' | 'meta' | 'paste' | 'copy' | 'chord';

interface KeyLike {
  code: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
}

export const isMetaCode = (code: string): boolean => code === 'MetaLeft' || code === 'MetaRight';
const isModifier = (code: string): boolean => /^(Control|Shift|Alt|Meta)(Left|Right)$/.test(code);

/**
 * pass: the client sends the key as it is. meta: the Cmd key of a Mac (kept from the computer: tapped alone it is the Windows key,
 * with another key it is Ctrl). paste: the local clipboard goes to the computer before the V. copy: the clipboard of the computer is
 * fetched after the C or X. chord: Cmd+key typed as Ctrl+key.
 */
export function planKey(e: KeyLike, mac: boolean): KeyPlan {
  if (mac && isMetaCode(e.code)) return 'meta';
  if (isModifier(e.code)) return 'pass';
  const viaMeta = mac && e.metaKey;
  if (e.altKey || !(viaMeta || e.ctrlKey)) return 'pass';
  if (!e.shiftKey && e.code === 'KeyV') return 'paste';
  if (!e.shiftKey && (e.code === 'KeyC' || e.code === 'KeyX')) return 'copy';
  return viaMeta ? 'chord' : 'pass';
}
