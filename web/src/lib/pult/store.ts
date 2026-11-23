import { get, writable } from 'svelte/store';

/** What a click on a server opens in the Console ("Пульт"): a terminal, the remote desktop (RDP) or the Assistant. */
export type PultMode = 'term' | 'rdp' | 'asst';

export interface PultTab {
  id: number;
  serverId: string;
  name: string;
  mode: PultMode;
}

/** The filters of the side list: the system and the state of the server. */
export interface PultFilter {
  os: 'all' | 'linux' | 'windows';
  status: 'all' | 'online' | 'offline';
}

const NO_FILTER: PultFilter = { os: 'all', status: 'all' };

interface Saved {
  mode: PultMode;
  tabs: PultTab[];
  active: number;
  collapsed: Record<string, boolean>;
  filter?: PultFilter;
}

function cleanFilter(v: unknown): PultFilter {
  const f = (v && typeof v === 'object' ? v : {}) as Partial<PultFilter>;
  return {
    os: f.os === 'linux' || f.os === 'windows' ? f.os : 'all',
    status: f.status === 'online' || f.status === 'offline' ? f.status : 'all',
  };
}

/** A server passes the filter: the system (windows or not) and the state match. */
export function passesFilter(f: PultFilter, isWindows: boolean, online: boolean): boolean {
  if (f.os === 'windows' && !isWindows) return false;
  if (f.os === 'linux' && isWindows) return false;
  if (f.status === 'online' && !online) return false;
  if (f.status === 'offline' && online) return false;
  return true;
}

const KEY = 'linager.pult.state';

function isMode(v: unknown): v is PultMode {
  return v === 'term' || v === 'rdp' || v === 'asst';
}

function read(): Saved {
  const empty: Saved = { mode: 'term', tabs: [], active: 0, collapsed: {}, filter: NO_FILTER };
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return empty;
    const v = JSON.parse(raw) as Partial<Saved>;
    const tabs = Array.isArray(v.tabs)
      ? v.tabs.filter((t): t is PultTab => !!t && typeof t.id === 'number' && typeof t.serverId === 'string' && typeof t.name === 'string' && isMode(t.mode))
      : [];
    const active = tabs.some((t) => t.id === v.active) ? (v.active as number) : (tabs[0]?.id ?? 0);
    return { mode: isMode(v.mode) ? v.mode : 'term', tabs, active, collapsed: v.collapsed && typeof v.collapsed === 'object' ? v.collapsed : {}, filter: cleanFilter(v.filter) };
  } catch {
    return empty; // storage unavailable or damaged: start empty
  }
}

const saved = read();

/** The mode of the side list: only the servers that have it are listed. */
export const pultMode = writable<PultMode>(saved.mode);
/** The open sessions, in the order they were opened. */
export const pultTabs = writable<PultTab[]>(saved.tabs);
/** The id of the tab on the screen (0: none). */
export const pultActive = writable<number>(saved.active);
/** The groups folded in the side list, by group name. */
export const pultCollapsed = writable<Record<string, boolean>>(saved.collapsed);
/** The filters of the side list (the system and the state of the server). */
export const pultFilter = writable<PultFilter>(saved.filter ?? NO_FILTER);
/** The Console is on the screen: the dock of the floating windows steps aside (the side list has the window list). */
export const pultShown = writable(false);

let nextId = saved.tabs.reduce((m, t) => Math.max(m, t.id), 0) + 1;

let timer: ReturnType<typeof setTimeout> | undefined;

/** Writes the state a moment after the last change (the stores change together). */
function persist(): void {
  clearTimeout(timer);
  timer = setTimeout(() => {
    try {
      localStorage.setItem(
        KEY,
        JSON.stringify({ mode: get(pultMode), tabs: get(pultTabs), active: get(pultActive), collapsed: get(pultCollapsed), filter: get(pultFilter) } satisfies Saved),
      );
    } catch {
      // storage unavailable: the state just is not remembered
    }
  }, 300);
}

pultMode.subscribe(persist);
pultTabs.subscribe(persist);
pultActive.subscribe(persist);
pultCollapsed.subscribe(persist);
pultFilter.subscribe(persist);

/** Opens a session of a server in the mode, or brings the open one to the front (one tab per server and mode). */
export function openPultTab(serverId: string, name: string, mode: PultMode): void {
  const open = get(pultTabs).find((t) => t.serverId === serverId && t.mode === mode);
  if (open) {
    pultActive.set(open.id);
    return;
  }
  const tab: PultTab = { id: nextId++, serverId, name, mode };
  pultTabs.update((list) => [...list, tab]);
  pultActive.set(tab.id);
}

/** Closes a tab; the one before it (or after, for the first) comes to the front when the tab was in front. */
export function closePultTab(id: number): void {
  const list = get(pultTabs);
  const index = list.findIndex((t) => t.id === id);
  if (index < 0) return;
  const next = list.filter((t) => t.id !== id);
  pultTabs.set(next);
  if (get(pultActive) === id) pultActive.set(next[Math.max(0, index - 1)]?.id ?? 0);
}

/** A server was renamed or removed: its tabs follow or go. */
export function syncPultServers(servers: { id: string; name: string }[]): void {
  const byId = new Map(servers.map((s) => [s.id, s.name]));
  const list = get(pultTabs);
  if (list.every((t) => byId.get(t.serverId) === t.name)) return;
  const next = list.filter((t) => byId.has(t.serverId)).map((t) => ({ ...t, name: byId.get(t.serverId) ?? t.name }));
  pultTabs.set(next);
  if (!next.some((t) => t.id === get(pultActive))) pultActive.set(next[0]?.id ?? 0);
}

export function closeAllPultTabs(): void {
  pultTabs.set([]);
  pultActive.set(0);
}
