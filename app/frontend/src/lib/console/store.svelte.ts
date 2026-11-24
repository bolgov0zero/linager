// The state of the Console: the mode of the side list, the open sessions (tabs), the folded groups and the filters.
// It is kept by the application (prefs), so that after a restart the work comes back.
import { getPref, setPref } from '../prefs';

/** What a click on a server opens: a terminal, the remote desktop (RDP) or the Assistant. */
export type ConMode = 'term' | 'rdp' | 'asst';

export interface ConTab {
  id: number;
  serverId: string;
  name: string;
  mode: ConMode;
}

export interface ConFilter {
  os: 'all' | 'linux' | 'windows';
  status: 'all' | 'online' | 'offline';
}

interface Saved {
  mode: ConMode;
  tabs: ConTab[];
  active: number;
  collapsed: Record<string, boolean>;
  filter: ConFilter;
}

const KEY = 'linager.console';
const isMode = (v: unknown): v is ConMode => v === 'term' || v === 'rdp' || v === 'asst';

function read(): Saved {
  const empty: Saved = { mode: 'term', tabs: [], active: 0, collapsed: {}, filter: { os: 'all', status: 'all' } };
  try {
    const raw = getPref(KEY);
    if (!raw) return empty;
    const v = JSON.parse(raw) as Partial<Saved>;
    const tabs = Array.isArray(v.tabs) ? v.tabs.filter((x): x is ConTab => !!x && typeof x.id === 'number' && typeof x.serverId === 'string' && typeof x.name === 'string' && isMode(x.mode)) : [];
    const f = (v.filter ?? {}) as Partial<ConFilter>;
    return {
      mode: isMode(v.mode) ? v.mode : 'term',
      tabs,
      active: tabs.some((x) => x.id === v.active) ? (v.active as number) : (tabs[0]?.id ?? 0),
      collapsed: v.collapsed && typeof v.collapsed === 'object' ? v.collapsed : {},
      filter: { os: f.os === 'linux' || f.os === 'windows' ? f.os : 'all', status: f.status === 'online' || f.status === 'offline' ? f.status : 'all' },
    };
  } catch {
    return empty;
  }
}

const saved = read();
export const con = $state<Saved & { seen: boolean }>({ ...saved, seen: false });
let seq = Math.max(0, ...saved.tabs.map((x) => x.id));
let timer: ReturnType<typeof setTimeout> | undefined;

function save(): void {
  clearTimeout(timer);
  timer = setTimeout(() => setPref(KEY, JSON.stringify({ mode: con.mode, tabs: con.tabs, active: con.active, collapsed: con.collapsed, filter: con.filter })), 300);
}

export function setMode(m: ConMode): void {
  con.mode = m;
  save();
}

/** Opens the session of a server in a mode, or brings the open one forward (one tab per server and mode). */
export function openTab(serverId: string, name: string, mode: ConMode): void {
  const have = con.tabs.find((x) => x.serverId === serverId && x.mode === mode);
  if (have) con.active = have.id;
  else {
    const tab = { id: ++seq, serverId, name, mode };
    con.tabs.push(tab);
    con.active = tab.id;
  }
  save();
}

export function closeTab(id: number): void {
  const i = con.tabs.findIndex((x) => x.id === id);
  if (i < 0) return;
  con.tabs.splice(i, 1);
  if (con.active === id) con.active = (con.tabs[i] ?? con.tabs[i - 1])?.id ?? 0;
  save();
}

export function activate(id: number): void {
  con.active = id;
  save();
}

export function toggleGroup(key: string): void {
  con.collapsed[key] = !con.collapsed[key];
  save();
}

export function setFilter(f: Partial<ConFilter>): void {
  Object.assign(con.filter, f);
  save();
}

/** A server passes the filter: the system and the state match. */
export function passes(f: ConFilter, windows: boolean, online: boolean): boolean {
  if (f.os === 'windows' && !windows) return false;
  if (f.os === 'linux' && windows) return false;
  if (f.status === 'online' && !online) return false;
  if (f.status === 'offline' && online) return false;
  return true;
}

/** The names of servers change: a tab keeps the name of its server. */
export function syncNames(names: Map<string, string>): void {
  let changed = false;
  for (const tab of con.tabs) {
    const n = names.get(tab.serverId);
    if (n && n !== tab.name) {
      tab.name = n;
      changed = true;
    }
  }
  if (changed) save();
}
