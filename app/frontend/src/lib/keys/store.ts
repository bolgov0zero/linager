// The open vault of the window. The decrypted entries live here, in memory, and nowhere else: not in the preferences, not
// on the server. A closed window, a restart or the auto-lock forgets them; the file is the only copy.
import { derived, get, writable } from 'svelte/store';
import { api, type Group } from '../api';
import { getPref, setPref } from '../prefs';
import { keyStateOf } from './state';
import { confirmAsk } from '../dialogs.svelte';
import { t } from '../i18n.svelte';
import { rememberedVaultFile, saveVaultFile, writeVaultFile, type Picked } from './vaultFile';
import { DEFAULT_FILE, VaultError, cleanEntry, newSession, openVault, sealVault, wipe, type Entry, type Session } from './vault';

export interface VaultState {
  /** The name of the file it was opened from, and the name a save is offered under. */
  file: string;
  entries: Entry[];
  /** Entries were changed and are not in the file yet (they are written a moment after the change). */
  dirty: boolean;
  /** The fingerprint of the file as it was last read or written ('' when not known): a write in place checks it. */
  sum: string;
  /** A write is going on. */
  saving: boolean;
  /** The last write failed and the entries are not in the file: the person has to act (the button "Save" shows). */
  saveError: boolean;
}

/** The vault is written into its file by itself after every change. False when that is not possible here (a browser in development): then "Save" is a button. */
export const autosave = writable(true);

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
let saveTimer: ReturnType<typeof setTimeout> | undefined;

// ---- auto-lock ----
/** Call on every use of the vault (a click in the window, a key picked in a form): the time of silence starts again. */
export function touch(): void {
  clearTimeout(timer);
  if (!get(vault)) return;
  const minutes = get(lockMinutes);
  if (minutes <= 0) return;
  timer = setTimeout(() => {
    void (async () => {
      if (!get(vault)) return;
      if (get(vault)?.dirty) await flush(); // what is not in the file yet is written first
      const state = get(vault);
      if (!state) return;
      if (state.dirty) lockWaiting.set(true);
      else closeVault();
    })();
  }, minutes * 60_000);
}

/** Forgets everything that was decrypted: the key and the entries. */
export function closeVault(): void {
  clearTimeout(timer);
  clearTimeout(saveTimer);
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
  const saved = await saveVaultFile(sealVault([], fresh), DEFAULT_FILE);
  if (saved === null) {
    wipe(fresh);
    return false;
  }
  closeVault();
  session = fresh;
  vault.set({ file: saved.name, entries: [], dirty: false, sum: saved.sum ?? '', saving: false, saveError: false });
  touch();
  return true;
}

/** Opens a picked file with the passphrase. Throws VaultError (`format`, `version`, `auth`, `limit`). */
export async function openPicked(picked: Picked, passphrase: string): Promise<void> {
  const opened = await openVault(picked.bytes, passphrase);
  closeVault();
  session = opened.session;
  vault.set({ file: picked.name || DEFAULT_FILE, entries: opened.entries, dirty: false, sum: picked.sum ?? '', saving: false, saveError: false });
  touch();
}

// ---- writing the file ----
// Every change is written into the open file a moment later, with no dialog and no button. The application writes it through a
// temporary file and keeps the previous content beside it as `<file>.bak`. When the file on the disk is not the one that was read
// (changed by a sync tool), nothing is overwritten without asking.
const SAVE_DELAY_MS = 500;
let writing: Promise<void> | null = null;

function patch(change: Partial<VaultState>): void {
  const v = get(vault);
  if (v) vault.set({ ...v, ...change });
}

/** Writes the entries as they are now; false when they are still not in the file (see `saveError`). */
async function persist(): Promise<boolean> {
  const state = get(vault);
  if (!state || !session) return true;
  patch({ saving: true });
  const bytes = sealVault(state.entries, session);
  let r = await writeVaultFile(bytes, state.sum);
  if (!r.ok && r.reason === 'changed') {
    // the file is not what was read: overwrite it, or keep it and save under another name
    const overwrite = await confirmAsk({ title: t('keys.conflict.title'), text: t('keys.conflict.text'), ok: t('keys.conflict.overwrite'), cancel: t('keys.conflict.other'), danger: true });
    if (overwrite) r = await writeVaultFile(bytes, state.sum, true);
    else {
      const saved = await saveVaultFile(bytes, state.file);
      r = saved ? { ok: true, name: saved.name, sum: saved.sum ?? '' } : { ok: false, reason: 'changed' };
    }
  } else if (!r.ok && (r.reason === 'gone' || r.reason === 'none')) {
    // no file to write into (deleted, or not remembered): the person says where it goes
    const saved = await saveVaultFile(bytes, state.file);
    r = saved ? { ok: true, name: saved.name, sum: saved.sum ?? '' } : { ok: false, reason: r.reason };
  }
  if (!r.ok && r.reason === 'unsupported') {
    autosave.set(false); // a browser: the button "Save" does it
    patch({ saving: false });
    return false;
  }
  const now = get(vault);
  if (!now) return r.ok;
  if (r.ok) {
    // changes that came while it was written stay to be written
    patch({ file: r.name, sum: r.sum, saving: false, saveError: false, dirty: now.entries !== state.entries });
    return now.entries === state.entries;
  }
  patch({ saving: false, saveError: true });
  return false;
}

/** Writes now what is not written yet (the writes go one after another). Resolves true when the file holds what the window holds. */
export async function flush(): Promise<boolean> {
  clearTimeout(saveTimer);
  if (!get(autosave)) return false;
  // a change that comes while a write goes on makes another round
  for (let round = 0; round < 50; round++) {
    if (writing) await writing;
    const v = get(vault);
    if (!v) return true;
    if (!v.dirty) return true;
    writing = persist().then(() => undefined).finally(() => (writing = null));
    await writing;
    if (get(vault)?.saveError || !get(autosave)) return false;
  }
  return !get(vault)?.dirty;
}

function scheduleSave(): void {
  if (!get(autosave)) return;
  clearTimeout(saveTimer);
  saveTimer = setTimeout(() => void flush(), SAVE_DELAY_MS);
}

/** "Save" as a button (a browser in development, or after a failure): writes, or offers the file for saving. */
export async function saveFile(): Promise<boolean> {
  const state = get(vault);
  if (!state || !session) return false;
  if (get(autosave)) {
    patch({ saveError: false });
    const ok = await flush();
    if (ok && get(lockWaiting)) closeVault();
    return ok;
  }
  const saved = await saveVaultFile(sealVault(state.entries, session), state.file);
  if (saved === null) return false;
  vault.set({ ...state, file: saved.name, dirty: false, sum: saved.sum ?? state.sum });
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
  scheduleSave();
  touch();
  return true;
}

export function removeEntry(id: string): void {
  const state = get(vault);
  if (!state) return;
  vault.set({ ...state, entries: state.entries.filter((e) => e.id !== id), dirty: true });
  scheduleSave();
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
