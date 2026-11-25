// The open vault of the window. The decrypted entries live here, in memory, and nowhere else: not in the preferences, not
// on the server. A closed window, a restart or the auto-lock forgets them; the file is the only copy.
import { derived, get, writable } from 'svelte/store';
import { api, type Group } from '../api';
import { getPref, setPref } from '../prefs';
import { keyStateOf } from './state';
import { rememberedVaultFile, saveVaultFile, type Picked } from './vaultFile';
import { DEFAULT_FILE, VaultError, cleanEntry, newSession, openVault, sealVault, wipe, type Entry, type Session } from './vault';

export interface VaultState {
  /** The name of the file it was opened from, and the name a save is offered under. */
  file: string;
  entries: Entry[];
  /** Entries were changed after the last save of the file. */
  dirty: boolean;
}

/** null while the vault is closed. */
export const vault = writable<VaultState | null>(null);

/** A vault is known to this window: one was opened here, or a file is remembered (the last one chosen in the last seven days). */
export const vaultKnown = writable(false);
vault.subscribe((v) => {
  if (v) vaultKnown.set(true);
});

/** Looks for a remembered file (the buttons show a red dot when a known vault is closed, and nothing when there is none). */
export async function checkVaultKnown(): Promise<void> {
  if (await rememberedVaultFile()) vaultKnown.set(true);
}

/** The remembered file was forgotten: a closed vault is not known any more. */
export function forgetKnown(): void {
  if (!get(vault)) vaultKnown.set(false);
}

/** The window "Keys" is open. A form (the RDP form) opens it from where it is, so the state is here and not in a component. */
export const keysOpen = writable(false);

/** The auto-lock time was reached but there are unsaved changes: the vault stays open until the file is saved. */
export const lockWaiting = writable(false);

/** "open", "waiting", "closed" or "none": the dot of the buttons of the keys (see state.ts). */
export const keyState = derived([vault, lockWaiting, vaultKnown], ([v, w, k]) => keyStateOf(v !== null, w, k));

/** Minutes of silence before the vault closes itself; 0 turns it off. */
export const LOCK_CHOICES = [5, 15, 30, 0] as const;
const LOCK_KEY = 'linager.keys.lock';

function savedLock(): number {
  const raw = getPref(LOCK_KEY);
  const n = Number(raw);
  return raw !== null && (LOCK_CHOICES as readonly number[]).includes(n) ? n : 15;
}

export const lockMinutes = writable<number>(savedLock());

export function setLockMinutes(minutes: number): void {
  lockMinutes.set(minutes);
  setPref(LOCK_KEY, String(minutes));
  touch();
}

let session: Session | null = null;
let timer: ReturnType<typeof setTimeout> | undefined;

// ---- auto-lock ----
/** Call on every use of the vault (a click in the window, a key picked in a form): the time of silence starts again. */
export function touch(): void {
  clearTimeout(timer);
  if (!get(vault)) return;
  const minutes = get(lockMinutes);
  if (minutes <= 0) return;
  timer = setTimeout(() => {
    const state = get(vault);
    if (!state) return;
    if (state.dirty) lockWaiting.set(true);
    else closeVault();
  }, minutes * 60_000);
}

/** Forgets everything that was decrypted: the key and the entries. */
export function closeVault(): void {
  clearTimeout(timer);
  wipe(session);
  session = null;
  lockWaiting.set(false);
  vault.set(null);
}

// ---- the file ----
/** A new empty vault with this passphrase; the file is offered for saving at once, so that it exists before anything is put in it.
 *  Returns false when the saving was cancelled (then nothing is created). */
export async function createVault(passphrase: string): Promise<boolean> {
  const fresh = await newSession(passphrase);
  const name = await saveVaultFile(sealVault([], fresh), DEFAULT_FILE);
  if (name === null) {
    wipe(fresh);
    return false;
  }
  closeVault();
  session = fresh;
  vault.set({ file: name, entries: [], dirty: false });
  touch();
  return true;
}

/** Opens a picked file with the passphrase. Throws VaultError (`format`, `version`, `auth`, `limit`). */
export async function openPicked(picked: Picked, passphrase: string): Promise<void> {
  const opened = await openVault(picked.bytes, passphrase);
  closeVault();
  session = opened.session;
  vault.set({ file: picked.name || DEFAULT_FILE, entries: opened.entries, dirty: false });
  touch();
}

/** Encrypts the entries again and offers the file for saving. */
export async function saveFile(): Promise<boolean> {
  const state = get(vault);
  if (!state || !session) return false;
  const name = await saveVaultFile(sealVault(state.entries, session), state.file);
  if (name === null) return false;
  vault.set({ ...state, file: name, dirty: false });
  if (get(lockWaiting)) closeVault();
  else touch();
  return true;
}

// ---- the entries ----
/** Adds an entry or replaces the one with the same id. Returns false when the entry is not valid. */
export function putEntry(entry: Entry): boolean {
  const state = get(vault);
  const clean = cleanEntry(entry);
  if (!state || !clean) return false;
  const at = state.entries.findIndex((e) => e.id === clean.id);
  const entries = at < 0 ? [...state.entries, clean] : state.entries.map((e, i) => (i === at ? clean : e));
  vault.set({ ...state, entries, dirty: true });
  touch();
  return true;
}

export function removeEntry(id: string): void {
  const state = get(vault);
  if (!state) return;
  vault.set({ ...state, entries: state.entries.filter((e) => e.id !== id), dirty: true });
  touch();
}

// ---- the servers and the groups of the panel (what a key can be bound to) ----
export interface DirServer {
  id: string;
  name: string;
  group_id: number | null;
}
export interface Directory {
  servers: DirServer[];
  /** The folders (with parent_id): a key bound to a folder is bound to everything under it. */
  groups: Group[];
}

let cached: { at: number; value: Directory } | null = null;

/** The servers and the groups of the panel. Kept for half a minute, so that a form and a window do not both ask. */
export async function loadDirectory(force = false): Promise<Directory> {
  if (!force && cached && Date.now() - cached.at < 30_000) return cached.value;
  const [d, groups] = await Promise.all([api.dashboard(), api.groups()]);
  cached = { at: Date.now(), value: { servers: d.items.map((c) => ({ id: c.id, name: c.name, group_id: c.group_id })), groups } };
  return cached.value;
}

export { VaultError };
