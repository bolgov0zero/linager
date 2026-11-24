// Pure logic of the "Edit container" dialog: the editable draft, the diff against the original, the validation.
// The rules of "on the spot" versus "recreate" are the agent's; the dialog repeats them for tags, the review step and the warning.

import type { DockerContainer, DockerEditConfig, DockerEditSpec } from './api';

export type EditTab = 'main' | 'ports' | 'mounts' | 'env' | 'res';
export type RestartPolicy = DockerEditSpec['restart'];
export type Proto = 'tcp' | 'udp';

/** Rows carry an `_id` so that the diff can tell a changed row from a removed one and an added one. */
export interface DraftPort { _id: number; ip: string; host: string; cont: string; proto: Proto }
export interface DraftMount { _id: number; type: 'bind' | 'volume'; src: string; dst: string; ro: boolean }
export interface DraftEnv { _id: number; k: string; v: string }

/** The form state: numbers are kept as the text the user typed. */
export interface Draft {
  name: string;
  image: string;
  restart: RestartPolicy;
  retries: string;
  cmd: string;
  mem: string;
  cpus: string;
  ports: DraftPort[];
  mounts: DraftMount[];
  env: DraftEnv[];
}

export type Translate = (key: string, vars?: Record<string, string | number>) => string;

let rowId = 0;
export function nextRowId(): number {
  return ++rowId;
}

/** The default number of retries offered when the policy becomes on-failure. */
const DEFAULT_RETRIES = '3';

export function toDraft(cfg: DockerEditConfig): Draft {
  return {
    name: cfg.name,
    image: cfg.image,
    restart: cfg.restart,
    retries: cfg.restart === 'on-failure' ? String(cfg.retries) : DEFAULT_RETRIES,
    cmd: cfg.cmd,
    mem: cfg.mem_mb > 0 ? String(cfg.mem_mb) : '',
    cpus: cfg.cpus > 0 ? String(cfg.cpus) : '',
    ports: (cfg.ports ?? []).map((p) => ({ _id: nextRowId(), ip: p.ip, host: String(p.host), cont: String(p.cont), proto: p.proto })),
    mounts: (cfg.mounts ?? []).map((m) => ({ _id: nextRowId(), type: m.type, src: m.src, dst: m.dst, ro: m.ro })),
    env: (cfg.env ?? []).map((e) => ({ _id: nextRowId(), k: e.k, v: e.v })),
  };
}

export function cloneDraft(d: Draft): Draft {
  return {
    ...d,
    ports: d.ports.map((r) => ({ ...r })),
    mounts: d.mounts.map((r) => ({ ...r })),
    env: d.env.map((r) => ({ ...r })),
  };
}

export function newPort(): DraftPort {
  return { _id: nextRowId(), ip: '0.0.0.0', host: '', cont: '', proto: 'tcp' };
}
export function newMount(): DraftMount {
  return { _id: nextRowId(), type: 'bind', src: '', dst: '', ro: false };
}
export function newEnv(): DraftEnv {
  return { _id: nextRowId(), k: '', v: '' };
}

// ---- numbers ----

/** A limit as a canonical string: '' and '0' are "no limit"; numbers lose leading zeros; anything else stays as typed. */
function normMem(s: string): string {
  const v = s.trim();
  if (v === '') return '0';
  return /^\d+$/.test(v) ? String(Number(v)) : v;
}

function normCpus(s: string): string {
  const v = s.trim().replace(',', '.');
  if (v === '') return '0';
  return /^\d*\.?\d+$/.test(v) ? String(Number(v)) : v;
}

function effRetries(restart: RestartPolicy, retries: string): string {
  if (restart !== 'on-failure') return '0';
  const v = retries.trim();
  return /^\d+$/.test(v) ? String(Number(v)) : v;
}

/** The request body. Call it only for a draft that passed `validateSpec`. */
export function toSpec(d: Draft): DockerEditSpec {
  return {
    name: d.name.trim(),
    image: d.image.trim(),
    restart: d.restart,
    retries: Number(effRetries(d.restart, d.retries)) || 0,
    cmd: d.cmd.trim(),
    mem_mb: Number(normMem(d.mem)) || 0,
    cpus: Number(normCpus(d.cpus)) || 0,
    ports: d.ports.map((p) => ({ ip: p.ip.trim(), host: Number(p.host.trim()), cont: Number(p.cont.trim()), proto: p.proto })),
    mounts: d.mounts.map((m) => ({ type: m.type, src: m.src.trim(), dst: m.dst.trim(), ro: m.ro })),
    env: d.env.map((e) => ({ k: e.k.trim(), v: e.v })),
  };
}

// ---- helpers for the command line and the environment ----

/** A variable whose value is hidden in the form and in the review. */
export function isSecretName(name: string): boolean {
  return /pass|secret|token|key/i.test(name);
}

/** Splits a command line into arguments: spaces separate, single and double quotes group, a backslash escapes in double quotes and outside quotes. Null if a quote is left open. */
export function splitCommand(line: string): string[] | null {
  const out: string[] = [];
  let cur = '';
  let has = false;
  let quote: '' | '"' | "'" = '';
  for (let i = 0; i < line.length; i++) {
    const ch = line[i];
    if (quote === "'") {
      if (ch === "'") quote = '';
      else cur += ch;
    } else if (quote === '"') {
      if (ch === '"') quote = '';
      else if (ch === '\\' && i + 1 < line.length && '"\\$`'.includes(line[i + 1])) cur += line[++i];
      else cur += ch;
    } else if (ch === '"' || ch === "'") {
      quote = ch;
      has = true;
    } else if (ch === '\\' && i + 1 < line.length) {
      cur += line[++i];
      has = true;
    } else if (/\s/.test(ch)) {
      if (has || cur) out.push(cur);
      cur = '';
      has = false;
    } else {
      cur += ch;
      has = true;
    }
  }
  if (quote) return null;
  if (has || cur) out.push(cur);
  return out;
}

/** Host ports taken by other containers: "80/tcp" -> the container's name. Parses "80→80/tcp" and ranges; "6379/tcp" (not published) is skipped. */
export function usedPorts(others: DockerContainer[]): Record<string, string> {
  const used: Record<string, string> = {};
  for (const c of others) {
    for (const text of c.ports ?? []) {
      for (const m of text.matchAll(/(\d+)(?:-(\d+))?(?:→|->)\d+(?:-\d+)?\/(tcp|udp)/g)) {
        const from = Number(m[1]);
        const to = m[2] ? Math.min(Number(m[2]), from + 2000) : from;
        for (let p = from; p <= to; p++) used[`${p}/${m[3]}`] = c.name;
      }
    }
  }
  return used;
}

// ---- diff ----

export interface Change {
  /** The tab that holds the field (for the dirty dot). */
  tab: EditTab;
  /** name, image, restart, retries, cmd, mem, cpus, port, mount, env: the caption is `dockerEdit.diff.<field>`. */
  field: string;
  /** a: added, d: removed, c: changed. */
  kind: 'a' | 'd' | 'c';
  before: string;
  after: string;
}

export interface DiffResult {
  changes: Change[];
  /** The container has to be recreated (otherwise everything is applied on the spot). */
  recreate: boolean;
  /** Tabs with a change. */
  tabs: Partial<Record<EditTab, boolean>>;
}

/** The words the diff puts into its values; the dialog passes the translated ones. */
export interface DiffWords {
  asImage: string;
  unlimited: string;
  mb: string;
  cores: string;
  readOnly: string;
  restart: Record<RestartPolicy, string>;
}

const EN_WORDS: DiffWords = {
  asImage: 'as in the image',
  unlimited: 'no limit',
  mb: 'MB',
  cores: 'cores',
  readOnly: 'read-only',
  restart: { no: 'no', always: 'always', 'unless-stopped': 'unless-stopped', 'on-failure': 'on-failure' },
};

export function portText(p: { ip: string; host: string; cont: string; proto: string }): string {
  return `${p.ip.trim()}:${p.host.trim()} → ${p.cont.trim()}/${p.proto}`;
}

export function mountText(m: { src: string; dst: string; ro: boolean }, words: DiffWords): string {
  return `${m.src.trim()} → ${m.dst.trim()}${m.ro ? ` (${words.readOnly})` : ''}`;
}

export function envText(e: { k: string; v: string }): string {
  return `${e.k.trim()}=${isSecretName(e.k) ? '••••••' : e.v}`;
}

/**
 * What differs between the original and the draft. Recreate: the image, the command, the ports, the mounts or the
 * environment changed, or a limit was removed. The name, the restart policy and a new or raised limit are applied on the spot.
 */
export function diffSpec(orig: Draft, draft: Draft, words: DiffWords = EN_WORDS): DiffResult {
  const changes: Change[] = [];
  const tabs: Partial<Record<EditTab, boolean>> = {};
  let recreate = false;

  function add(tab: EditTab, field: string, kind: Change['kind'], before: string, after: string, re: boolean): void {
    changes.push({ tab, field, kind, before, after });
    tabs[tab] = true;
    if (re) recreate = true;
  }

  const name = (s: string) => s.trim();
  if (name(orig.name) !== name(draft.name)) add('main', 'name', 'c', name(orig.name), name(draft.name), false);
  if (name(orig.image) !== name(draft.image)) add('main', 'image', 'c', name(orig.image), name(draft.image), true);
  if (orig.restart !== draft.restart) add('main', 'restart', 'c', words.restart[orig.restart], words.restart[draft.restart], false);
  if (draft.restart === 'on-failure' && effRetries(orig.restart, orig.retries) !== effRetries(draft.restart, draft.retries)) {
    add('main', 'retries', 'c', effRetries(orig.restart, orig.retries), effRetries(draft.restart, draft.retries), false);
  }
  if (name(orig.cmd) !== name(draft.cmd)) add('main', 'cmd', 'c', name(orig.cmd) || words.asImage, name(draft.cmd) || words.asImage, true);

  const limit = (v: string, unit: string) => (v === '0' ? words.unlimited : `${v} ${unit}`);
  const om = normMem(orig.mem);
  const dm = normMem(draft.mem);
  if (om !== dm) add('res', 'mem', 'c', limit(om, words.mb), limit(dm, words.mb), dm === '0');
  const oc = normCpus(orig.cpus);
  const dc = normCpus(draft.cpus);
  if (oc !== dc) add('res', 'cpus', 'c', limit(oc, words.cores), limit(dc, words.cores), dc === '0');

  function rows<R extends { _id: number }>(tab: EditTab, field: string, before: R[], after: R[], key: (r: R) => string, text: (r: R) => string): void {
    for (const b of before) {
      const a = after.find((x) => x._id === b._id);
      if (!a) add(tab, field, 'd', text(b), '', true);
      else if (key(a) !== key(b)) add(tab, field, 'c', text(b), text(a), true);
    }
    for (const a of after) {
      if (!before.some((x) => x._id === a._id)) add(tab, field, 'a', '', text(a), true);
    }
  }
  rows('ports', 'port', orig.ports, draft.ports, portText, portText);
  rows('mounts', 'mount', orig.mounts, draft.mounts, (m) => `${m.type}|${mountText(m, words)}`, (m) => mountText(m, words));
  // The key holds the real value, the text hides a secret: a changed password is still a change.
  rows('env', 'env', orig.env, draft.env, (e) => `${e.k.trim()}=${e.v}`, envText);

  return { changes, recreate, tabs };
}

// ---- validation ----

export interface ValidateOpts {
  /** The form as it was loaded: rows and fields that are unchanged are not checked again. */
  orig: Draft;
  /** A compose container: only the restart policy and the limits are checked (the rest is read-only). */
  compose: boolean;
  /** Host ports of the other containers: see `usedPorts`. */
  others: Record<string, string>;
  t: Translate;
}

const NAME_RE = /^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$/;
const IMAGE_RE = /^[a-zA-Z0-9][a-zA-Z0-9_./:@-]{0,255}$/;
const VOLUME_RE = /^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/;
const ENV_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;
const IPV4_RE = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;

function validPort(s: string): boolean {
  const v = s.trim();
  return /^\d+$/.test(v) && Number(v) >= 1 && Number(v) <= 65535;
}

function validIp(s: string): boolean {
  const v = s.trim();
  const m = IPV4_RE.exec(v);
  if (m) return m.slice(1).every((o) => Number(o) <= 255);
  return v.includes(':') && /^[0-9a-fA-F:.]+$/.test(v);
}

function absolute(s: string): boolean {
  return /^\/[^\0]*$/.test(s.trim());
}

/** The tab that holds the field of an error path. */
export function tabOfPath(path: string): EditTab {
  const head = path.split('.')[0];
  if (head === 'ports' || head === 'env') return head;
  if (head === 'mounts') return 'mounts';
  if (head === 'mem' || head === 'cpus') return 'res';
  return 'main';
}

/**
 * Path -> message. Paths: name, image, cmd, retries, mem, cpus, and `ports.<_id>.ip|host|cont`, `mounts.<_id>.src|dst`,
 * `env.<_id>.k`. The first message in the map is the first problem in the order of the tabs.
 */
export function validateSpec(draft: Draft, opts: ValidateOpts): Record<string, string> {
  const { orig, compose, others, t } = opts;
  const errs: Record<string, string> = {};
  const bad = (path: string, msg: string): void => {
    errs[path] = msg;
  };

  if (!compose) {
    if (draft.name.trim() !== orig.name.trim() && !NAME_RE.test(draft.name.trim())) bad('name', t('dockerEdit.err.name'));
    if (draft.image.trim() !== orig.image.trim() && !IMAGE_RE.test(draft.image.trim())) bad('image', t('dockerEdit.err.image'));
    if (draft.cmd.trim() !== orig.cmd.trim() && splitCommand(draft.cmd) === null) bad('cmd', t('dockerEdit.err.cmd'));
  }
  if (draft.restart === 'on-failure' && !/^\d+$/.test(draft.retries.trim())) bad('retries', t('dockerEdit.err.retries'));

  if (!compose) {
    const seen: Record<string, boolean> = {};
    for (const p of draft.ports) {
      const before = orig.ports.find((o) => o._id === p._id);
      const same = !!before && portText(before) === portText(p) && before.proto === p.proto;
      const pre = `ports.${p._id}`;
      const hostOk = validPort(p.host);
      if (!same) {
        if (!hostOk) bad(`${pre}.host`, t('dockerEdit.err.hostPort'));
        if (!validPort(p.cont)) bad(`${pre}.cont`, t('dockerEdit.err.contPort'));
        if (!validIp(p.ip)) bad(`${pre}.ip`, t('dockerEdit.err.ip'));
      }
      if (hostOk) {
        const port = Number(p.host.trim());
        const key = `${port}/${p.proto}`;
        if (!same && others[key]) bad(`${pre}.host`, t('dockerEdit.err.portBusy', { port, name: others[key] }));
        else if (seen[key]) bad(`${pre}.host`, t('dockerEdit.err.portDup', { port }));
        seen[key] = true;
      }
    }

    const dsts: Record<string, boolean> = {};
    for (const m of draft.mounts) {
      const before = orig.mounts.find((o) => o._id === m._id);
      const same = !!before && before.type === m.type && before.src.trim() === m.src.trim() && before.dst.trim() === m.dst.trim();
      const pre = `mounts.${m._id}`;
      const dst = m.dst.trim();
      if (!same) {
        if (m.type === 'bind' && !absolute(m.src)) bad(`${pre}.src`, t('dockerEdit.err.bindSrc'));
        if (m.type === 'volume' && !VOLUME_RE.test(m.src.trim())) bad(`${pre}.src`, t('dockerEdit.err.volName'));
        if (!absolute(dst)) bad(`${pre}.dst`, t('dockerEdit.err.dst'));
      }
      if (absolute(dst)) {
        if (dsts[dst] && !errs[`${pre}.dst`]) bad(`${pre}.dst`, t('dockerEdit.err.dstDup', { path: dst }));
        dsts[dst] = true;
      }
    }

    const names: Record<string, boolean> = {};
    for (const e of draft.env) {
      const before = orig.env.find((o) => o._id === e._id);
      const same = !!before && before.k.trim() === e.k.trim();
      const k = e.k.trim();
      if (!same && !ENV_RE.test(k)) bad(`env.${e._id}.k`, t('dockerEdit.err.envName'));
      else if (names[k]) bad(`env.${e._id}.k`, t('dockerEdit.err.envDup', { name: k }));
      names[k] = true;
    }
  }

  const mem = draft.mem.trim();
  if (normMem(mem) !== normMem(orig.mem) && mem !== '' && (!/^\d+$/.test(mem) || (Number(mem) !== 0 && Number(mem) < 6))) bad('mem', t('dockerEdit.err.mem'));
  const cpus = draft.cpus.trim().replace(',', '.');
  if (normCpus(cpus) !== normCpus(orig.cpus) && cpus !== '' && (!/^\d*\.?\d+$/.test(cpus) || Number(cpus) < 0 || Number(cpus) > 128)) bad('cpus', t('dockerEdit.err.cpus'));

  return errs;
}
