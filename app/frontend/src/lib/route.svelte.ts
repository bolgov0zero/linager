import { getPref, setPref } from './prefs';

// The page of the main window, kept in the hash: "" is the dashboard, "#/s/<server id>/<tab>" a server.
export interface Route {
  page: 'dash' | 'server';
  id: string;
  tab: string;
}

function parse(): Route {
  const m = /^#\/s\/([^/?]+)(?:\/([\w-]+))?/.exec(location.hash);
  return m ? { page: 'server', id: decodeURIComponent(m[1]), tab: m[2] ?? 'overview' } : { page: 'dash', id: '', tab: '' };
}

export const route = $state<Route>(parse());

// The section of the top bar, remembered between starts.
export type Section = 'dash' | 'console';
const SKEY = 'linager.section';
export const nav = $state<{ section: Section }>({ section: getPref(SKEY) === 'console' ? 'console' : 'dash' });

export function setSection(s: Section): void {
  nav.section = s;
  setPref(SKEY, s);
  if (route.page === 'server') openDashboard();
}

window.addEventListener('hashchange', () => Object.assign(route, parse()));

export function openServer(id: string, tab = 'overview'): void {
  location.hash = `#/s/${encodeURIComponent(id)}/${tab}`;
}

export function openDashboard(): void {
  location.hash = '';
  history.replaceState(null, '', location.pathname + location.search);
  Object.assign(route, parse());
}
