// Helpers of the address lists (nftables sets) of the firewall draft.

/** An IPv4 address or network: 10.0.0.1 or 10.0.0.0/24. */
export function isIPv4(value: string): boolean {
  const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})(?:\/(\d{1,2}))?$/.exec(value);
  if (!m) return false;
  if (![m[1], m[2], m[3], m[4]].every((x) => Number(x) <= 255)) return false;
  return m[5] === undefined || Number(m[5]) <= 32;
}

/** An IPv6 address or network, checked loosely; the agent validates it again with nft. */
export function isIPv6(value: string): boolean {
  const [addr, prefix, extra] = value.split('/');
  if (extra !== undefined || !addr.includes(':') || !/^[0-9a-fA-F:]+$/.test(addr)) return false;
  if ((addr.match(/::/g) ?? []).length > 1 || /:::/.test(addr)) return false;
  if (addr.split(':').some((g) => g.length > 4)) return false;
  return prefix === undefined || (/^\d{1,3}$/.test(prefix) && Number(prefix) <= 128);
}

/** Splits pasted text into addresses (separated by spaces, commas, semicolons or new lines) of a family. */
export function parseAddresses(text: string, family: 'ipv4' | 'ipv6'): { items: string[]; bad: string[] } {
  const items: string[] = [];
  const bad: string[] = [];
  for (const raw of text.split(/[\s,;]+/)) {
    const x = raw.trim();
    if (!x) continue;
    if (family === 'ipv4' ? isIPv4(x) : isIPv6(x)) {
      if (!items.includes(x)) items.push(x);
    } else {
      bad.push(x);
    }
  }
  return { items, bad };
}

/** Number of rules that refer to the list `@name`. */
export function usedIn(rules: string[], name: string): number {
  const re = new RegExp(`@${name}(?![a-z0-9_])`);
  return rules.filter((r) => re.test(r)).length;
}
