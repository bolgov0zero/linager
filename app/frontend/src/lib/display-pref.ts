// What the application remembers about the window of a remote screen, per server (kept in the preferences of the application, never on
// the Linager server) and about the sidebar of the page of a server (one choice for all the servers).
export type AsstDisplay = 'fit' | 'fill' | 'real';
export type RdpScale = 'fit' | 'real';

export const asstDisplayKey = (serverId: string): string => `linager.asst.display.${serverId}`;
export const rdpScaleKey = (serverId: string): string => `linager.rdp.scale.${serverId}`;
export const rdpAutoKey = (serverId: string): string => `linager.rdp.auto.${serverId}`;
/** One key for every server: a folded sidebar is folded on all the pages of servers. */
export const SIDEBAR_KEY = 'linager.sp.collapsed';

export const parseAsstDisplay = (v: string | null): AsstDisplay => (v === 'fill' || v === 'real' ? v : 'fit');
export const parseRdpScale = (v: string | null): RdpScale => (v === 'real' ? 'real' : 'fit');
/** A flag stored as "1" or "0"; anything else (nothing stored) is the default. */
export const parseFlag = (v: string | null, fallback: boolean): boolean => (v === '1' ? true : v === '0' ? false : fallback);
export const flagValue = (on: boolean): string => (on ? '1' : '0');
