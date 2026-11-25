// The state of the vault as the buttons of the keys show it (a dot on the corner of the button): green when it is open, amber when the
// auto-lock time came but there is a file not saved yet (it stays open until it is saved), red when it is closed. A person who has
// never opened a vault (no file is remembered, none was opened in this window) sees no dot at all: there are no keys to be locked.
export type KeyState = 'open' | 'waiting' | 'closed' | 'none';

export function keyStateOf(open: boolean, waiting: boolean, known: boolean): KeyState {
  if (open) return waiting ? 'waiting' : 'open';
  return known ? 'closed' : 'none';
}
