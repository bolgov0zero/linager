/** A point of a bar chart: average bar plus a peak mark. */
export interface ChartPoint {
  ts: number; // unix seconds, start of the bar
  value: number;
  peak: number;
}

/** A marker on the time axis (reboot, service failure, ...). */
export interface ChartEvent {
  ts: number; // unix seconds
  kind: string;
}

/** One client-side live sample; null when the value is unknown. */
export interface LiveSample {
  cpu: number | null;
  mem: number | null;
  rx: number | null;
  tx: number | null;
}

/** Number of live samples kept for the overview bars. */
export const HISTORY_LIMIT = 60;

export const TABS = [
  { id: 'overview', key: 'srv.tab.overview' },
  { id: 'metrics', key: 'srv.tab.metrics' },
  { id: 'terminal', key: 'srv.tab.terminal' },
  { id: 'desktop', key: 'srv.tab.desktop' },
  { id: 'assistant', key: 'srv.tab.assistant' },
  { id: 'files', key: 'srv.tab.files' },
  { id: 'disk', key: 'srv.tab.disk' },
  { id: 'services', key: 'srv.tab.services' },
  { id: 'processes', key: 'srv.tab.processes' },
  { id: 'programs', key: 'srv.tab.programs' },
  { id: 'network', key: 'srv.tab.network' },
  { id: 'firewall', key: 'srv.tab.firewall' },
  { id: 'security', key: 'srv.tab.security' },
  { id: 'manage', key: 'srv.tab.manage' },
  { id: 'users', key: 'srv.tab.users' },
  { id: 'docker', key: 'srv.tab.docker' },
  { id: 'updates', key: 'srv.tab.updates' },
  { id: 'logs', key: 'srv.tab.logs' },
] as const;

export type TabId = (typeof TABS)[number]['id'];

/** Agent module that backs a tab; tabs not listed here (overview, metrics) are always shown. */
const TAB_MODULE: Partial<Record<TabId, string>> = {
  terminal: 'terminal',
  desktop: 'desktop',
  assistant: 'assistant',
  manage: 'assistant',
  files: 'files',
  disk: 'disk',
  services: 'services',
  network: 'network',
  firewall: 'firewall',
  security: 'security',
  users: 'users',
  docker: 'docker',
  updates: 'apt',
  processes: 'processes',
  programs: 'programs',
  logs: 'logs',
};

/**
 * Tabs to show for the reported agent modules. An empty list means the agent did not report
 * its modules (old agent): all tabs stay except Docker, Security, Disk, Users and the Windows-only Assistant, Manage and Programs, which need the module to be reported.
 */
export function visibleTabs(modules: readonly string[], allowed: readonly string[] | null = null): (typeof TABS)[number][] {
  return TABS.filter((tab) => {
    const module = TAB_MODULE[tab.id];
    if (!module) return true;
    // Windows Update has its own agent module behind the same tab
    if (tab.id === 'updates' && modules.includes('winupdate') && (allowed === null || allowed.includes('apt'))) return true;
    // an operator only gets the modules that were given (an administrator: null), an auditor none
    if (allowed !== null && !allowed.includes(module)) return false;
    if (modules.length === 0) return module !== 'docker' && module !== 'security' && module !== 'disk' && module !== 'users' && module !== 'assistant' && module !== 'programs';
    return modules.includes(module);
  });
}

/** Sidebar groups of the server page; every tab belongs to exactly one group, in the order shown. */
export const TAB_GROUPS: readonly { id: string; key: string; tabs: readonly TabId[] }[] = [
  { id: 'general', key: 'srv.group.general', tabs: ['overview', 'metrics', 'terminal', 'desktop', 'assistant'] },
  { id: 'system', key: 'srv.group.system', tabs: ['services', 'processes', 'programs', 'logs', 'updates'] },
  { id: 'fs', key: 'srv.group.fs', tabs: ['files', 'disk'] },
  { id: 'net', key: 'srv.group.net', tabs: ['network', 'firewall', 'security', 'manage', 'users'] },
  { id: 'apps', key: 'srv.group.apps', tabs: ['docker'] },
];

/** The visible tabs split into the sidebar groups; a group with no visible tab is dropped. */
export function groupTabs(tabs: readonly (typeof TABS)[number][]): { id: string; key: string; tabs: (typeof TABS)[number][] }[] {
  return TAB_GROUPS.map((g) => ({
    id: g.id,
    key: g.key,
    tabs: g.tabs.flatMap((id) => tabs.filter((t) => t.id === id)),
  })).filter((g) => g.tabs.length > 0);
}

/**
 * The agent modules split into the same groups as the sidebar, in the order of its tabs (a module that backs two tabs is listed
 * once, in the first group). It is what the list of modules of an operator is built from, so the two never disagree.
 */
export function moduleGroups(): { id: string; key: string; modules: string[] }[] {
  const seen = new Set<string>();
  return TAB_GROUPS.map((g) => ({
    id: g.id,
    key: g.key,
    modules: g.tabs.flatMap((id) => {
      const m = TAB_MODULE[id];
      if (!m || seen.has(m)) return [];
      seen.add(m);
      return [m];
    }),
  })).filter((g) => g.modules.length > 0);
}

/** 9x9 pixel icons of the tabs, one string per row; "1" is a filled square. Drawn so that one square is 2 px at 18 px, which keeps them sharp on any screen. */
