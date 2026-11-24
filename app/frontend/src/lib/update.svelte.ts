// Self-update of the application: the state the settings dialog and the top bar show. The checking and the installing
// are done by the application (internal/update); the page asks it and follows the progress.
import { getPref, setPref } from './prefs';

export interface UpdateInfo {
  supported: boolean;
  reason?: string;
  current: string;
  channel: string;
  latest?: string;
  newer: boolean;
  notes?: string[];
  size?: number;
  checked_at?: number;
}

export interface UpdateStatus {
  phase: 'idle' | 'downloading' | 'installing' | 'restarting' | 'error';
  pct: number;
  version?: string;
  error?: string;
}

/** Why the last check failed: GitHub refused for the number of requests (retryAt: unix time it may be asked again), or something else. */
export interface UpdateProblem { kind: 'rate_limited' | 'network' | 'other'; retryAt?: number }

export const upd = $state<{ info: UpdateInfo | null; status: UpdateStatus; checking: boolean; failed: boolean; problem: UpdateProblem | null }>({
  info: null,
  status: { phase: 'idle', pct: 0 },
  checking: false,
  failed: false,
  problem: null,
});

const AUTO = 'linager.update.auto';
const CHANNEL = 'linager.update.channel';
/** The channel the person chose; '' follows the running version (a beta follows the betas, a release the releases). */
export const chosenChannel = (): string => {
  const v = getPref(CHANNEL);
  return v === 'stable' || v === 'beta' ? v : '';
};
export const setChannel = (c: 'stable' | 'beta'): void => setPref(CHANNEL, c);
const channelQuery = (): string => (chosenChannel() ? `?channel=${chosenChannel()}` : '');

export const autoCheck = (): boolean => getPref(AUTO) !== '0';
export const setAutoCheck = (on: boolean): void => setPref(AUTO, on ? '1' : '0');

async function json<T>(method: string, path: string): Promise<{ ok: boolean; data: T }> {
  const res = await fetch(`app/update/${path}`, { method });
  const text = await res.text();
  return { ok: res.ok, data: (text ? JSON.parse(text) : {}) as T };
}

/** The GitHub token of the checks (kept by the application in the keychain; the page never gets it back). */
export const githubToken = {
  /** Whether a token is kept. */
  get: async (): Promise<boolean> => (await json<{ set?: boolean }>('GET', 'token')).data.set === true,
  /** Checks the token with GitHub and keeps it; resolves with an error code ('invalid_token', 'bad_token', 'network') or ''. */
  set: async (token: string): Promise<string> => {
    const res = await fetch('app/update/token', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ token }) });
    if (res.ok) return '';
    const body = (await res.json().catch(() => ({}))) as { error?: string };
    return body.error ?? 'network';
  },
  clear: async (): Promise<void> => void (await fetch('app/update/token', { method: 'DELETE' })),
};

/** Asks the application to look for a new version. */
export async function checkUpdate(): Promise<void> {
  if (upd.checking) return;
  upd.checking = true;
  try {
    const r = await json<UpdateInfo & { info?: UpdateInfo; problem?: UpdateProblem['kind']; retry_at?: number }>('POST', `check${channelQuery()}`);
    upd.failed = !r.ok;
    upd.problem = r.ok ? null : { kind: r.data.problem ?? 'network', retryAt: r.data.retry_at };
    upd.info = r.ok ? r.data : (r.data.info ?? upd.info);
  } catch {
    upd.failed = true;
    upd.problem = { kind: 'network' };
  } finally {
    upd.checking = false;
  }
}

let timer: ReturnType<typeof setInterval> | undefined;
let follow: ReturnType<typeof setInterval> | undefined;

/** Starts the checks: once soon after the start, then every six hours, unless the person turned them off. */
export function startAutoCheck(): () => void {
  void json<UpdateInfo>('GET', `info${channelQuery()}`).then((r) => (upd.info = r.data)).catch(() => undefined);
  const run = (): void => {
    if (autoCheck() && upd.info?.supported) void checkUpdate();
  };
  const first = setTimeout(run, 8000);
  clearInterval(timer);
  timer = setInterval(run, 6 * 3600 * 1000);
  return () => {
    clearTimeout(first);
    clearInterval(timer);
  };
}

/** Starts the installation and follows it until the application restarts or fails. */
export async function installUpdate(): Promise<void> {
  upd.status = { phase: 'downloading', pct: 0, version: upd.info?.latest };
  const r = await json<{ error?: string }>('POST', 'install');
  if (!r.ok) {
    upd.status = { phase: 'error', pct: 0, error: r.data.error ?? 'error' };
    return;
  }
  clearInterval(follow);
  follow = setInterval(async () => {
    try {
      const s = await json<UpdateStatus>('GET', 'status');
      upd.status = s.data;
      if (s.data.phase === 'error') clearInterval(follow);
    } catch {
      // the application is restarting: the page goes away with it
    }
  }, 500);
}

export const updateBusy = (): boolean => ['downloading', 'installing', 'restarting'].includes(upd.status.phase);
