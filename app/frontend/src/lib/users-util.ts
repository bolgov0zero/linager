import type { HostUser, HostUsersState } from './api';

/** Groups that give (almost) the access of root; they are marked in the lists. */
export const HOT_GROUPS: readonly string[] = ['sudo', 'wheel', 'docker', 'lxd', 'disk'];

const ALPHABET = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789-_.+=';

/** A random password from the browser's own generator (no look-alike characters). Never leaves the page except to be set. */
export function genPassword(length = 20): string {
  const limit = 256 - (256 % ALPHABET.length); // rejection sampling: no modulo bias
  const out: string[] = [];
  while (out.length < length) {
    const bytes = crypto.getRandomValues(new Uint8Array(length * 2));
    for (const b of bytes) {
      if (b < limit && out.length < length) out.push(ALPHABET[b % ALPHABET.length]);
    }
  }
  return out.join('');
}

export interface UserFilter {
  human: boolean;
  system: boolean;
  sudo: boolean;
  online: boolean;
  q: string;
}

export function filterUsers(users: readonly HostUser[], f: UserFilter): HostUser[] {
  const q = f.q.trim().toLowerCase();
  const byKind = f.human || f.system;
  return users.filter((u) => {
    if (byKind && !((f.human && u.human) || (f.system && !u.human))) return false;
    if (f.sudo && u.sudo === 'none') return false;
    if (f.online && u.sessions === 0) return false;
    if (q && !`${u.name} ${u.gecos} ${u.shell} ${u.groups.join(' ')}`.toLowerCase().includes(q)) return false;
    return true;
  });
}

/**
 * True when the account is the only unlocked user with sudo and root cannot log in: taking its rights or
 * locking it would leave nobody to administer the host (the agent refuses it as well).
 */
export function lastAdmin(state: HostUsersState, name: string): boolean {
  if (state.root_login) return false;
  return !state.users.some((u) => u.name !== name && u.human && u.sudo !== 'none' && !u.locked);
}

/** Tone of the sudo badge: a passwordless sudo is the dangerous one. */
export function sudoTone(s: HostUser['sudo']): 'muted' | 'warn' | 'crit' {
  return s === 'none' ? 'muted' : s === 'pw' ? 'warn' : 'crit';
}
