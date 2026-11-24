import type { SecState } from './api';

export type Tone = 'ok' | 'warn' | 'crit' | 'muted';

/** One line of the state overview: a label key, the value key (with variables) and how worrying it is. */
export interface Check {
  id: string;
  label: string;
  value: string;
  vars?: Record<string, string | number>;
  tone: Tone;
}

interface Sample {
  security_updates?: number;
  reboot_required?: boolean;
}

/** PermitRootLogin: only "yes" lets root in with a password. */
function rootCheck(v: string): Check {
  const label = 'sec.chk.root';
  switch (v) {
    case 'yes':
      return { id: 'root', label, value: 'sec.root.yes', tone: 'crit' };
    case 'no':
      return { id: 'root', label, value: 'sec.root.no', tone: 'ok' };
    case 'prohibit-password':
    case 'without-password':
      return { id: 'root', label, value: 'sec.root.key', tone: 'ok' };
    case 'forced-commands-only':
      return { id: 'root', label, value: 'sec.root.forced', tone: 'ok' };
    default:
      return { id: 'root', label, value: 'sec.unknown', tone: 'muted' };
  }
}

/** The security overview of a host. Passwords and an empty password are the worst, a missing ban tool is a warning. */
export function postureChecks(sec: SecState, sample: Sample | null | undefined): Check[] {
  const out: Check[] = [];
  if (sec.ssh.installed) {
    out.push({ id: 'port', label: 'sec.chk.port', value: 'sec.port', vars: { ports: sec.ssh.ports.join(', ') }, tone: 'muted' });
    out.push(rootCheck(sec.ssh.permit_root_login));
    out.push(
      sec.ssh.password_auth === 'no'
        ? { id: 'password', label: 'sec.chk.password', value: 'sec.password.no', tone: 'ok' }
        : sec.ssh.password_auth === 'yes'
          ? { id: 'password', label: 'sec.chk.password', value: 'sec.password.yes', tone: 'warn' }
          : { id: 'password', label: 'sec.chk.password', value: 'sec.unknown', tone: 'muted' },
    );
    out.push(
      sec.ssh.pubkey_auth === 'no'
        ? { id: 'pubkey', label: 'sec.chk.pubkey', value: 'sec.pubkey.no', tone: 'warn' }
        : { id: 'pubkey', label: 'sec.chk.pubkey', value: 'sec.pubkey.yes', tone: 'ok' },
    );
    if (sec.ssh.permit_empty_passwords === 'yes') {
      out.push({ id: 'empty', label: 'sec.chk.empty', value: 'sec.empty.yes', tone: 'crit' });
    }
    out.push(
      sec.ssh.user_restrictions
        ? { id: 'allow', label: 'sec.chk.allow', value: 'sec.allow.yes', tone: 'ok' }
        : { id: 'allow', label: 'sec.chk.allow', value: 'sec.allow.no', tone: 'muted' },
    );
  }
  const f = sec.fail2ban;
  if (!f.installed) out.push({ id: 'f2b', label: 'sec.chk.f2b', value: 'sec.f2b.none', tone: 'warn' });
  else if (!f.active) out.push({ id: 'f2b', label: 'sec.chk.f2b', value: 'sec.f2b.stopped', tone: 'warn' });
  else {
    const banned = f.jails.reduce((n, j) => n + j.currently_banned, 0);
    out.push({ id: 'f2b', label: 'sec.chk.f2b', value: 'sec.f2b.on', vars: { jails: f.jails.length, banned }, tone: 'ok' });
  }
  out.push(
    sec.auto_updates
      ? { id: 'auto', label: 'sec.chk.auto', value: 'sec.auto.on', tone: 'ok' }
      : { id: 'auto', label: 'sec.chk.auto', value: 'sec.auto.off', tone: 'warn' },
  );
  const pending = sample?.security_updates ?? -1;
  if (pending > 0) out.push({ id: 'secupd', label: 'sec.chk.secupd', value: 'sec.secupd.n', vars: { n: pending }, tone: 'warn' });
  else if (pending === 0) out.push({ id: 'secupd', label: 'sec.chk.secupd', value: 'sec.secupd.none', tone: 'ok' });
  if (sample?.reboot_required) out.push({ id: 'reboot', label: 'sec.chk.reboot', value: 'sec.reboot.yes', tone: 'warn' });
  return out;
}

/** The worst tone of the checks, for the heading of the overview. */
export function worstTone(checks: Check[]): Tone {
  if (checks.some((c) => c.tone === 'crit')) return 'crit';
  if (checks.some((c) => c.tone === 'warn')) return 'warn';
  return 'ok';
}

/** How many checks need the administrator's attention (warnings and critical ones; "muted" is only informational). */
export function issueCount(checks: Check[]): number {
  return checks.filter((c) => c.tone === 'crit' || c.tone === 'warn').length;
}

const SSH_IDS = new Set(['port', 'root', 'password', 'pubkey', 'empty', 'allow', 'f2b']);

/** The checks in two groups for the state block: everything about SSH access, and the system itself. */
export function groupChecks(checks: Check[]): { ssh: Check[]; system: Check[] } {
  return { ssh: checks.filter((c) => SSH_IDS.has(c.id)), system: checks.filter((c) => !SSH_IDS.has(c.id)) };
}

/** The locale key of the short "what to do" line for a check, or null when it is fine (or only informational). */
export function hintKey(c: Check): string | null {
  if (c.tone === 'ok') return null;
  if (c.id === 'allow') return c.tone === 'muted' ? 'sec.hint.allow' : null;
  if (c.tone === 'muted') return null;
  return `sec.hint.${c.id}`;
}

/** Height of the bar of a day in percent of the busiest day (at least a visible stub for a non-zero day). */
export function barHeights(byDay: number[]): number[] {
  const max = Math.max(1, ...byDay);
  return byDay.map((n) => (n === 0 ? 0 : Math.max(6, Math.round((n / max) * 100))));
}

/** Human key type: "RSA 2048", "ED25519". */
export function keyLabel(type: string, bits?: number): string {
  if (type === 'ssh-ed25519') return 'ED25519';
  if (type === 'ssh-rsa') return bits ? `RSA ${bits}` : 'RSA';
  if (type === 'ssh-dss') return 'DSA';
  if (type.startsWith('ecdsa-sha2-')) return `ECDSA ${type.slice('ecdsa-sha2-'.length)}`;
  if (type.startsWith('sk-')) return `FIDO ${type.slice(3).replace('ssh-', '')}`;
  return type;
}
