// The file of the vault (Keys): opened and saved through the native dialogs of the application; in a browser (development)
// through a file field and a download link. The bytes are encrypted already: nothing readable passes here.
import { native, NativeError } from '../native';
import { FILE_EXT } from './vault';

export interface Picked {
  name: string;
  bytes: Uint8Array;
  /** The fingerprint of the file as read (the application's): a write in place is refused when the file is another by then. */
  sum?: string;
}

function fromBase64(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function toBase64(bytes: Uint8Array): string {
  let bin = '';
  for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(bin);
}

function pickInBrowser(): Promise<Picked | null> {
  return new Promise((resolve) => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = FILE_EXT;
    input.onchange = async () => {
      const f = input.files?.[0];
      resolve(f ? { name: f.name, bytes: new Uint8Array(await f.arrayBuffer()) } : null);
    };
    input.oncancel = () => resolve(null);
    input.click();
  });
}

/** The file chosen earlier (the application remembers it for seven days of use); null when there is none. */
export async function rememberedVaultFile(): Promise<Picked | null> {
  try {
    const r = await native.vaultLast();
    return r.data ? { name: r.name ?? '', bytes: fromBase64(r.data), sum: r.sum } : null;
  } catch {
    return null; // a browser in development, or an old application
  }
}

export const forgetVaultFile = (): Promise<unknown> => native.vaultForget().catch(() => undefined);

/** Asks for a vault file; null when it was cancelled. */
export async function pickVaultFile(): Promise<Picked | null> {
  try {
    const r = await native.vaultOpen();
    return r.cancelled || !r.data ? null : { name: r.name ?? '', bytes: fromBase64(r.data), sum: r.sum };
  } catch (e) {
    if (e instanceof NativeError && e.status === 501) return pickInBrowser();
    throw e;
  }
}

export interface Saved {
  name: string;
  sum?: string;
}

/** Offers the bytes to be saved as a file; returns the name it was saved under (and its fingerprint), or null when it was cancelled. */
export async function saveVaultFile(bytes: Uint8Array, name: string): Promise<Saved | null> {
  try {
    const r = await native.vaultSave(name, toBase64(bytes));
    return r.cancelled ? null : { name: r.name ?? name, sum: r.sum };
  } catch (e) {
    if (!(e instanceof NativeError && e.status === 501)) throw e;
  }
  const url = URL.createObjectURL(new Blob([bytes as BlobPart], { type: 'application/octet-stream' }));
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
  return { name };
}

export type WriteResult = { ok: true; name: string; sum: string } | { ok: false; reason: 'changed' | 'gone' | 'none' | 'unsupported' | 'failed' };

/**
 * Writes the bytes into the file that is open, with no dialog (the application keeps a copy of the previous content beside it). Says
 * why when it could not: `changed` (the file on the disk is not what was read: not overwritten unless `force`), `gone` (deleted),
 * `none` (no file is remembered), `unsupported` (a browser, in development) or `failed`.
 */
export async function writeVaultFile(bytes: Uint8Array, base: string, force = false): Promise<WriteResult> {
  try {
    const r = await native.vaultWrite(toBase64(bytes), base, force);
    return { ok: true, name: r.name, sum: r.sum };
  } catch (e) {
    if (e instanceof NativeError) {
      if (e.status === 409) return { ok: false, reason: 'changed' };
      if (e.status === 404) return { ok: false, reason: e.code === 'gone' ? 'gone' : 'none' };
      if (e.status === 501) return { ok: false, reason: 'unsupported' };
    }
    return { ok: false, reason: 'failed' };
  }
}
