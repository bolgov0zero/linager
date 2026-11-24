// The personal vault of the logins and passwords ("Keys"): one encrypted file the person keeps, opened in the browser.
// Nothing here talks to the panel. The file is Argon2id (the key from the passphrase) plus AES-256-GCM (the content);
// the header is authenticated, so a changed cost or salt is caught as a wrong file. Pure JavaScript (@noble): it works
// on plain HTTP too, where `crypto.subtle` is not there, and it needs no WebAssembly.
//
// File layout (big endian):
//   0   4  magic "LGRK"
//   4   1  format version (1)
//   5   1  key derivation (1 = Argon2id)
//   6   4  memory, KiB
//   10  4  passes
//   14  4  lanes
//   18 16  salt
//   34 12  nonce
//   46  .  AES-GCM of the JSON content, with the 16 bytes of the tag at the end; the bytes 0..46 are the associated data

import { argon2idAsync } from '@noble/hashes/argon2.js';
import { gcm } from '@noble/ciphers/aes.js';

export type EntryType = 'rdp' | 'uac' | 'any';

/** A group or a server the key is bound to: its id in the panel and its name when it was saved (for the list, if it is gone). */
export interface Ref {
  id: string;
  name: string;
}

/** One saved login. It may be bound to several groups and several servers; with none of them it is for all servers. */
export interface Entry {
  id: string;
  name: string;
  type: EntryType;
  groups: Ref[];
  servers: Ref[];
  login: string;
  domain: string;
  password: string;
  note: string;
}

export interface KdfParams {
  /** Memory in KiB. */
  m: number;
  /** Passes. */
  t: number;
  /** Lanes. */
  p: number;
}

/** What stays in memory while the vault is open: the key, so that saving does not run the slow derivation again. */
export interface Session {
  key: Uint8Array;
  salt: Uint8Array;
  params: KdfParams;
}

export type VaultErrorCode = 'format' | 'version' | 'auth' | 'limit';

export class VaultError extends Error {
  readonly code: VaultErrorCode;
  constructor(code: VaultErrorCode, message?: string) {
    super(message ?? code);
    this.name = 'VaultError';
    this.code = code;
  }
}

export const DEFAULT_PARAMS: KdfParams = { m: 65536, t: 3, p: 1 };
export const PASSPHRASE_MIN = 10;
export const MAX_FILE = 1 << 20;
export const MAX_ENTRIES = 500;
export const LIMITS = { name: 80, login: 256, domain: 256, password: 512, note: 1000, ref: 64, refs: 100 } as const;
export const FILE_EXT = '.lvault';
export const DEFAULT_FILE = 'linager-keys.lvault';

const MAGIC = [0x4c, 0x47, 0x52, 0x4b]; // "LGRK"
const VERSION = 1;
const KDF_ARGON2ID = 1;
const HEADER = 46;
const SALT = 16;
const NONCE = 12;
const TAG = 16;
// What a file may ask for when it is opened: a file from elsewhere must not be able to eat the memory of the tab.
const MAX_M = 262144;
const MIN_M = 8192;
const MAX_T = 10;
const MAX_P = 4;

const TYPES: readonly EntryType[] = ['rdp', 'uac', 'any'];

function random(n: number): Uint8Array {
  const out = new Uint8Array(n);
  crypto.getRandomValues(out);
  return out;
}

/** A new id of an entry. */
export function newId(): string {
  return Array.from(random(8), (b) => b.toString(16).padStart(2, '0')).join('');
}

/** An empty entry for the form. */
export function blankEntry(): Entry {
  return { id: newId(), name: '', type: 'rdp', groups: [], servers: [], login: '', domain: '', password: '', note: '' };
}

function text(v: unknown, max: number): string | null {
  return typeof v === 'string' && v.length <= max ? v : null;
}

function refs(v: unknown): Ref[] | null {
  if (v === undefined) return [];
  if (!Array.isArray(v) || v.length > LIMITS.refs) return null;
  const out: Ref[] = [];
  for (const r of v) {
    if (typeof r !== 'object' || r === null) return null;
    const id = text((r as Record<string, unknown>).id, LIMITS.ref);
    const name = text((r as Record<string, unknown>).name ?? '', LIMITS.name);
    if (!id || name === null || out.some((x) => x.id === id)) return null;
    out.push({ id, name });
  }
  return out;
}

/** Checks one entry of a file or of the form. Returns null when it is not an entry. */
export function cleanEntry(x: unknown): Entry | null {
  if (typeof x !== 'object' || x === null) return null;
  const o = x as Record<string, unknown>;
  const id = text(o.id, 32);
  const name = text(o.name, LIMITS.name);
  const login = text(o.login, LIMITS.login);
  const domain = text(o.domain ?? '', LIMITS.domain);
  const password = text(o.password ?? '', LIMITS.password);
  const note = text(o.note ?? '', LIMITS.note);
  const groups = refs(o.groups);
  const servers = refs(o.servers);
  if (!id || name === null || login === null || domain === null || password === null || note === null || groups === null || servers === null) return null;
  if (!TYPES.includes(o.type as EntryType)) return null;
  return { id, name, type: o.type as EntryType, groups, servers, login, domain, password, note };
}

/** A passphrase is compared in one Unicode form, so that the same word typed on another keyboard opens the same file. */
function pass(passphrase: string): Uint8Array {
  return new TextEncoder().encode(passphrase.normalize('NFKC'));
}

async function derive(passphrase: string, salt: Uint8Array, params: KdfParams): Promise<Uint8Array> {
  const bytes = pass(passphrase);
  try {
    return await argon2idAsync(bytes, salt, { t: params.t, m: params.m, p: params.p, dkLen: 32, asyncTick: 10 });
  } finally {
    bytes.fill(0);
  }
}

/** A new vault: a new salt and the key from the passphrase. */
export async function newSession(passphrase: string, params: KdfParams = DEFAULT_PARAMS): Promise<Session> {
  const salt = random(SALT);
  return { key: await derive(passphrase, salt, params), salt, params: { ...params } };
}

/** Forgets the key. */
export function wipe(session: Session | null): void {
  session?.key.fill(0);
}

function header(session: Session, nonce: Uint8Array): Uint8Array {
  const h = new Uint8Array(HEADER);
  const v = new DataView(h.buffer);
  h.set(MAGIC, 0);
  h[4] = VERSION;
  h[5] = KDF_ARGON2ID;
  v.setUint32(6, session.params.m);
  v.setUint32(10, session.params.t);
  v.setUint32(14, session.params.p);
  h.set(session.salt, 18);
  h.set(nonce, 34);
  return h;
}

/** The bytes of the file for these entries. A fresh nonce every time. */
export function sealVault(entries: Entry[], session: Session): Uint8Array {
  if (entries.length > MAX_ENTRIES) throw new VaultError('limit');
  const plain = new TextEncoder().encode(JSON.stringify({ v: VERSION, entries }));
  try {
    const nonce = random(NONCE);
    const head = header(session, nonce);
    const sealed = gcm(session.key, nonce, head).encrypt(plain);
    const out = new Uint8Array(HEADER + sealed.length);
    out.set(head, 0);
    out.set(sealed, HEADER);
    if (out.length > MAX_FILE) throw new VaultError('limit');
    return out;
  } finally {
    plain.fill(0);
  }
}

/** Opens a file with the passphrase. Throws VaultError: `format` (not a vault file), `version`, `auth` (wrong phrase or a changed file), `limit`. */
export async function openVault(bytes: Uint8Array, passphrase: string): Promise<{ entries: Entry[]; session: Session }> {
  if (bytes.length > MAX_FILE) throw new VaultError('limit');
  if (bytes.length < HEADER + TAG || MAGIC.some((b, i) => bytes[i] !== b)) throw new VaultError('format');
  if (bytes[4] !== VERSION || bytes[5] !== KDF_ARGON2ID) throw new VaultError('version');
  const v = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const params = { m: v.getUint32(6), t: v.getUint32(10), p: v.getUint32(14) };
  if (params.m < MIN_M || params.m > MAX_M || params.t < 1 || params.t > MAX_T || params.p < 1 || params.p > MAX_P) throw new VaultError('format');
  const head = bytes.slice(0, HEADER);
  const salt = head.slice(18, 18 + SALT);
  const nonce = head.slice(34, 34 + NONCE);
  const key = await derive(passphrase, salt, params);
  let plain: Uint8Array;
  try {
    plain = gcm(key, nonce, head).decrypt(bytes.slice(HEADER));
  } catch {
    key.fill(0);
    throw new VaultError('auth');
  }
  try {
    const data = JSON.parse(new TextDecoder().decode(plain)) as { v?: unknown; entries?: unknown };
    if (data.v !== VERSION) throw new VaultError('version');
    if (!Array.isArray(data.entries)) throw new VaultError('format');
    if (data.entries.length > MAX_ENTRIES) throw new VaultError('limit');
    const entries: Entry[] = [];
    for (const raw of data.entries) {
      const e = cleanEntry(raw);
      if (!e) throw new VaultError('format');
      entries.push(e);
    }
    return { entries, session: { key, salt, params } };
  } catch (e) {
    key.fill(0);
    if (e instanceof VaultError) throw e;
    throw new VaultError('format');
  } finally {
    plain.fill(0);
  }
}

// ---- the lists for a form ----

/** The server the form is opened for. */
export interface Target {
  serverId: string;
  /** The id of its folder, null when it has none. */
  groupId: number | null;
  /** The folder and the folders above it (a key bound to any of them is for this server). Without it only `groupId` counts. */
  groupChain?: number[];
}

/** The servers and the groups the panel knows now: a key bound only to things that are gone is "not found". */
export interface Known {
  servers: ReadonlySet<string>;
  groups: ReadonlySet<string>;
}

/** The key is bound, and every group and server it is bound to is gone from the panel. */
export function isLost(e: Entry, known: Known): boolean {
  if (e.groups.length + e.servers.length === 0) return false;
  return !e.groups.some((g) => known.groups.has(g.id)) && !e.servers.some((s) => known.servers.has(s.id));
}

/** Where a key is for all servers (no binding at all). */
export const isForAll = (e: Entry): boolean => e.groups.length + e.servers.length === 0;

export interface Sections {
  /** Bound to this server. */
  here: Entry[];
  /** Bound to the group of this server. */
  group: Entry[];
  /** For all servers; a binding that points only at things that are gone counts as this. */
  all: Entry[];
  /** Bound to other servers and groups. */
  other: Entry[];
}

/** The keys for a form, in the order shown: this server, its group, all, then the rest. `use` keeps the type that fits ("any" fits both). */
export function sectionsFor(entries: readonly Entry[], target: Target, known: Known, use: 'rdp' | 'uac'): Sections {
  const out: Sections = { here: [], group: [], all: [], other: [] };
  const chain = target.groupChain ?? (target.groupId !== null ? [target.groupId] : []);
  for (const e of entries) {
    if (e.type !== 'any' && e.type !== use) continue;
    if (isForAll(e) || isLost(e, known)) out.all.push(e);
    else if (e.servers.some((s) => s.id === target.serverId)) out.here.push(e);
    else if (chain.some((id) => e.groups.some((g) => g.id === String(id)))) out.group.push(e);
    else out.other.push(e);
  }
  const byName = (a: Entry, b: Entry) => a.name.localeCompare(b.name);
  for (const list of [out.here, out.group, out.all, out.other]) list.sort(byName);
  return out;
}

/** "CORP\user", or just the login when there is no domain. */
export function accountOf(e: Pick<Entry, 'login' | 'domain'>): string {
  return e.domain ? `${e.domain}\\${e.login}` : e.login;
}
