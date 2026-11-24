// A readable view of an nft rule: action, protocol, port, source and the rest.

export type ViewAction = 'accept' | 'drop' | 'reject' | 'other';

export interface RuleView {
  action: ViewAction;
  /** tcp, udp, icmp, ... in upper case, or '' for "any". */
  proto: string;
  /** 22 or "80, 443"; '' when the rule has no port. */
  port: string;
  /** An address, a network or @list; '' for "any". */
  source: string;
  /** An interface the rule is limited to (iif / iifname). */
  iface: string;
  /** The comment of the rule. */
  comment: string;
  /** What did not fit the columns: connection state, limits, flags, destination. */
  extra: string;
  /** The rule is marked for the firewall journal (it carries the `lg:` log prefix). */
  logged: boolean;
}

/** The log part the panel writes (nft prints `burst 5 packets` after the rate when it lists the rule). */
const LOG_RE = /(?:\blimit\s+rate\s+\S+(?:\s+burst\s+\d+\s+\w+)?\s+)?\blog\s+prefix\s+"lg:[^"]*"(?:\s+level\s+\w+)?/;
const LOG_RATE = '5/second';

const SYSTEM_RE = /^ct state (?:\{\s*)?established,\s*related(?:\s*\})?(?: counter(?: packets \d+ bytes \d+)?)? accept$/;

/** The rule that lets the replies of existing connections through; the agent puts it first in every chain. */
export function isSystemRule(text: string): boolean {
  return SYSTEM_RE.test(text.trim().replace(/\s+/g, ' '));
}

function list(value: string): string {
  return value
    .replace(/[{}]/g, '')
    .split(',')
    .map((x) => x.trim())
    .filter(Boolean)
    .join(', ');
}

/** Splits an nft rule into the columns of the table. Anything unknown stays in `extra`, nothing is lost. */
export function parseRule(text: string): RuleView {
  let t = ` ${text.trim().replace(/\s+/g, ' ')} `;
  const take = (re: RegExp): RegExpMatchArray | null => {
    const m = t.match(re);
    if (m) t = t.replace(re, ' ');
    return m;
  };

  const comment = take(/\bcomment "([^"]*)"/)?.[1] ?? '';
  const logged = take(LOG_RE) !== null;
  take(/\bcounter(?: packets \d+ bytes \d+)?\b/);
  const verdict = take(/\b(accept|drop|reject)\b/)?.[1] as ViewAction | undefined;

  let proto = '';
  let port = '';
  const dport = take(/\b(tcp|udp)\s+dport\s+(\{[^}]*\}|\S+)/);
  if (dport) {
    proto = dport[1];
    port = list(dport[2]);
  } else {
    const l4 = take(/\bmeta\s+l4proto\s+(\{[^}]*\}|\S+)/) ?? take(/\bip6?\s+protocol\s+(\{[^}]*\}|\S+)/) ?? take(/\bip6\s+nexthdr\s+(\{[^}]*\}|\S+)/);
    if (l4) proto = list(l4[1]);
  }

  let source = '';
  const saddr = take(/\bip6?\s+saddr\s+(\{[^}]*\}|\S+)/);
  if (saddr) source = list(saddr[1]);
  const iface = take(/\biif(?:name)?\s+"?([\w.*-]+)"?/)?.[1] ?? '';

  const state = take(/\bct\s+state\s+(\{[^}]*\}|\S+)/);
  let extra = t.trim();
  if (state) extra = `ct state ${list(state[1]).replace(/, /g, ',')}${extra ? ' · ' + extra : ''}`;

  return { action: verdict ?? 'other', proto: proto.toUpperCase(), port, source, iface, comment, extra, logged };
}

/** The rule without the log part, blanks collapsed. */
export function stripLog(text: string): string {
  return text.replace(LOG_RE, ' ').trim().replace(/\s+/g, ' ');
}

export function isLogged(text: string): boolean {
  return LOG_RE.test(text);
}

/** A short stable id of a rule (its text without the log part and the counter): the log prefix carries it. */
export function ruleLogId(text: string): string {
  const norm = stripLog(text).replace(/\bcounter(?: packets \d+ bytes \d+)?/g, '').replace(/\s+/g, '');
  let h = 0x811c9dc5;
  for (let i = 0; i < norm.length; i++) {
    h ^= norm.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return (h >>> 0).toString(16).padStart(8, '0').slice(0, 4);
}

const LETTER: Record<string, string> = { accept: 'A', drop: 'D', reject: 'R' };

/** Whether the rule has a verdict the journal can show. */
export function canLog(text: string): boolean {
  return /\b(accept|drop|reject)\b/.test(stripLog(text));
}

/** Adds the log part (before the counter or the verdict) or removes it. */
export function setLogged(text: string, chain: string, on: boolean): string {
  const base = stripLog(text);
  if (!on) return base;
  const verdict = base.match(/\b(accept|drop|reject)\b/);
  if (!verdict) return base;
  const part = `limit rate ${LOG_RATE} log prefix "lg:${chain}:${ruleLogId(base)}:${LETTER[verdict[1]]} "`;
  const at = base.search(/\bcounter\b/);
  const pos = at >= 0 ? at : (verdict.index ?? 0);
  return `${base.slice(0, pos)}${part} ${base.slice(pos)}`.replace(/\s+/g, ' ').trim();
}

function toBig(ip: string): { v: bigint; bits: number } | null {
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(ip)) {
    const p = ip.split('.').map(Number);
    if (p.some((x) => x > 255)) return null;
    return { v: p.reduce((a, x) => (a << 8n) | BigInt(x), 0n), bits: 32 };
  }
  if (!ip.includes(':') || !/^[0-9a-fA-F:]+$/.test(ip)) return null;
  const [head, tail, extra] = ip.split('::');
  if (extra !== undefined) return null;
  const h = head ? head.split(':') : [];
  const t = tail !== undefined && tail ? tail.split(':') : [];
  if (tail === undefined && h.length !== 8) return null;
  const fill = tail === undefined ? [] : Array(8 - h.length - t.length).fill('0');
  const groups = [...h, ...fill, ...t];
  if (groups.length !== 8 || groups.some((g) => g.length > 4)) return null;
  return { v: groups.reduce((a, g) => (a << 16n) | BigInt(parseInt(g || '0', 16)), 0n), bits: 128 };
}

/** Whether an address belongs to one of the addresses or networks (a, a.b.c.d/24, 2001:db8::/32). */
export function ipInList(ip: string, elements: string[]): boolean {
  const a = toBig(ip);
  if (!a) return false;
  for (const e of elements) {
    const [addr, len] = e.split('/');
    const b = toBig(addr);
    if (!b || b.bits !== a.bits) continue;
    const n = len === undefined ? b.bits : Number(len);
    if (!Number.isInteger(n) || n < 0 || n > b.bits) continue;
    const shift = BigInt(b.bits - n);
    if (a.v >> shift === b.v >> shift) return true;
  }
  return false;
}
