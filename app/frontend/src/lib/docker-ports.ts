/** Published ports without the bind address (IPv4 and IPv6 give the same pair twice), each pair once. */
export function portList(ports: string[] | null | undefined): string[] {
  const seen = new Set<string>();
  for (const p of ports ?? []) {
    const s = p.replace(/^(\[[0-9a-fA-F:]*\]|[0-9.]+|::):(?=\d)/, '').trim();
    if (s) seen.add(s);
  }
  return [...seen];
}

/** The ports cell: the first pair and "+N" for the rest (the full list goes to the tooltip). */
export function portsShort(ports: string[] | null | undefined): string {
  const l = portList(ports);
  if (l.length === 0) return '—';
  return l.length === 1 ? l[0] : `${l[0]} +${l.length - 1}`;
}
