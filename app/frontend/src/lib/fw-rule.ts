// Builds nft rule text from the "Добавить правило" form. Returns a locale key on error.

export type RuleProto = 'tcp' | 'udp' | 'icmp' | 'any';
export type RuleAction = 'accept' | 'drop' | 'reject';

export interface RuleForm {
  proto: RuleProto;
  port: string;
  source: string;
  action: RuleAction;
  comment: string;
}

/** The name and family of an address list a rule can refer to. */
export interface ListRef {
  name: string;
  family: 'ipv4' | 'ipv6';
}

export type BuildResult = { ok: true; text: string } | { ok: false; error: string };

const PORTS_RE = /^\d{1,5}(,\d{1,5})*$/;
const SOURCE_RE = /^[0-9a-fA-F:.]+(\/\d{1,3})?$/;
const LIST_RE = /^@[a-z][a-z0-9_]{0,30}$/;

/** Collapses whitespace so the rule text matches what the server reports. */
export function normalizeRule(text: string): string {
  return text.trim().replace(/\s+/g, ' ');
}

/**
 * Order: [ip saddr <source or @list>] [<proto> dport <port or { a, b }>] counter <action> [comment "<text>"].
 * For icmp the match is "ip protocol icmp" and no port is allowed.
 */
export function buildRule(form: RuleForm, lists: ListRef[] = []): BuildResult {
  const port = form.port.replace(/\s+/g, '');
  const source = form.source.trim();
  const comment = form.comment.trim().replace(/"/g, '');

  if ((form.proto === 'icmp' || form.proto === 'any') && port) {
    return { ok: false, error: 'fw.add.errPortProto' };
  }
  if (port && !PORTS_RE.test(port)) return { ok: false, error: 'fw.add.errPortFormat' };
  const ports = port ? port.split(',') : [];
  if (ports.some((p) => Number(p) < 1 || Number(p) > 65535)) return { ok: false, error: 'fw.add.errPortRange' };
  let saddr = '';
  if (source.startsWith('@')) {
    if (!LIST_RE.test(source)) return { ok: false, error: 'fw.add.errSource' };
    const list = lists.find((l) => l.name === source.slice(1));
    if (!list) return { ok: false, error: 'fw.add.errList' };
    saddr = `${list.family === 'ipv6' ? 'ip6' : 'ip'} saddr ${source}`;
  } else if (source) {
    if (!SOURCE_RE.test(source)) return { ok: false, error: 'fw.add.errSource' };
    saddr = `${source.includes(':') ? 'ip6' : 'ip'} saddr ${source}`;
  }
  if (form.proto === 'any' && !source && !comment) return { ok: false, error: 'fw.add.errEmpty' };

  const parts: string[] = [];
  if (saddr) parts.push(saddr);
  if (form.proto === 'tcp' || form.proto === 'udp') {
    if (ports.length === 0) {
      parts.push(`meta l4proto ${form.proto}`);
    } else {
      const value = ports.length > 1 ? `{ ${ports.join(', ')} }` : ports[0];
      parts.push(`${form.proto} dport ${value}`);
    }
  } else if (form.proto === 'icmp') {
    parts.push('ip protocol icmp');
  }
  parts.push(`counter ${form.action}`);
  if (comment) parts.push(`comment "${comment}"`);
  return { ok: true, text: parts.join(' ') };
}
