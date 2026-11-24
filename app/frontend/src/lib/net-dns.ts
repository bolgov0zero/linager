/** Client-side checks for the DNS editor; the server and the agent repeat them. */

export const DNS_MAX_SERVERS = 3;
export const DNS_MAX_SEARCH = 6;
const SEARCH_LINE_MAX = 256;

const LABEL = /^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$/;

export function isIPv4(s: string): boolean {
  const p = s.split('.');
  return p.length === 4 && p.every((x) => /^\d{1,3}$/.test(x) && Number(x) <= 255);
}

export function isIPv6(s: string): boolean {
  if (!s.includes(':') || !/^[0-9A-Fa-f:.]+$/.test(s)) return false;
  try {
    new URL(`http://[${s}]/`);
    return true;
  } catch {
    return false;
  }
}

export function isHostname(s: string): boolean {
  return s.length > 0 && s.length <= 253 && s.split('.').every((l) => LABEL.test(l));
}

/** Search domains typed separated by spaces, commas or semicolons; a trailing dot is dropped, case is folded. */
export function parseSearch(text: string): string[] {
  const out: string[] = [];
  for (const raw of text.split(/[\s,;]+/)) {
    const d = raw.replace(/\.$/, '').toLowerCase();
    if (d && !out.includes(d)) out.push(d);
  }
  return out;
}

export interface DnsCheck {
  /** One entry per row: an i18n key of the problem or null. */
  rows: (string | null)[];
  /** An i18n key for the list as a whole (count) or null. */
  list: string | null;
  search: string | null;
  /** The cleaned lists to send. */
  nameservers: string[];
  domains: string[];
  ok: boolean;
}

export function checkDns(rows: string[], searchText: string): DnsCheck {
  const rowErr = rows.map((r) => {
    const v = r.trim();
    if (v === '') return null;
    return isIPv4(v) || isIPv6(v) ? null : 'net.dns.err.ip';
  });
  const ns: string[] = [];
  for (const r of rows) {
    const v = r.trim();
    if (v !== '' && !ns.includes(v)) ns.push(v);
  }
  let list: string | null = null;
  if (ns.length < 1) list = 'net.dns.err.none';
  else if (ns.length > DNS_MAX_SERVERS) list = 'net.dns.err.count';
  const domains = parseSearch(searchText);
  let search: string | null = null;
  if (domains.some((d) => !isHostname(d))) search = 'net.dns.err.domain';
  else if (domains.length > DNS_MAX_SEARCH) search = 'net.dns.err.domains';
  else if (domains.length > 0 && `search ${domains.join(' ')}`.length > SEARCH_LINE_MAX) search = 'net.dns.err.long';
  const ok = rowErr.every((e) => e === null) && list === null && search === null;
  return { rows: rowErr, list, search, nameservers: ns, domains, ok };
}
