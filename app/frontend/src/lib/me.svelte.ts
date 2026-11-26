// Who is signed in: the role and, for an operator, the modules that were given. The server decides what is allowed, but the
// interface does not offer what would be refused (the buttons of an administrator, the tabs of modules that were not given).
import type { Role, Session } from './api';

export const me = $state<{ role: Role; modules: string[] }>({ role: 'admin', modules: [] });

export function setMe(s: Session | null): void {
  me.role = s?.role ?? 'admin';
  me.modules = s?.modules ?? [];
}

export const isAdmin = (): boolean => me.role === 'admin';
/** An administrator has every module, an operator the ones given, an auditor none. */
export const hasModule = (m: string): boolean => me.role === 'admin' || (me.role === 'operator' && me.modules.includes(m));

/** The agent module that backs a tab of the server page; the tabs that are not listed (overview, metrics) are always shown. */
const TAB_MODULE: Record<string, string> = {
  terminal: 'terminal', desktop: 'desktop', assistant: 'assistant', manage: 'assistant', files: 'files', disk: 'disk',
  services: 'services', network: 'network', firewall: 'firewall', security: 'security', users: 'users', docker: 'docker',
  updates: 'apt', processes: 'processes', programs: 'programs', logs: 'logs',
};

/**
 * Whether a tab is offered: for the modules the agent reports and for the rights of the person. An empty list of the agent's
 * modules means an old agent that did not report them: everything stays except what needs the module to be reported.
 */
export function tabVisible(key: string, agentModules: readonly string[]): boolean {
  const module = TAB_MODULE[key];
  if (!module) return true;
  // Windows Update has its own agent module behind the same tab
  if (key === 'updates' && agentModules.includes('winupdate') && hasModule('apt')) return true;
  if (!hasModule(module)) return false;
  if (agentModules.length === 0) return !['docker', 'security', 'disk', 'users', 'assistant', 'programs', 'manage'].includes(key);
  return agentModules.includes(module);
}

/** Devices that wait for an administrator's decision (the mark on the button of the settings, the toast). */
export const devs = $state({ pending: 0 });
