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

export const upd = $state<{ info: UpdateInfo | null; status: UpdateStatus; checking: boolean; failed: boolean }>({
  info: null,
  status: { phase: 'idle', pct: 0 },
  checking: false,
  failed: false,
});

const AUTO = 'linager.update.auto';
export const autoCheck = (): boolean => getPref(AUTO) !== '0';
export const setAutoCheck = (on: boolean): void => setPref(AUTO, on ? '1' : '0');

async function json<T>(method: string, path: string): Promise<{ ok: boolean; data: T }> {
  const res = await fetch(`app/update/${path}`, { method });
  const text = await res.text();
  return { ok: res.ok, data: (text ? JSON.parse(text) : {}) as T };
}

/** Asks the application to look for a new version. */
export async function checkUpdate(): Promise<void> {
  if (upd.checking) return;
  upd.checking = true;
  try {
    const r = await json<UpdateInfo & { info?: UpdateInfo }>('POST', 'check');
    upd.failed = !r.ok;
    upd.info = r.ok ? r.data : (r.data.info ?? upd.info);
  } catch {
    upd.failed = true;
  } finally {
    upd.checking = false;
  }
}

let timer: ReturnType<typeof setInterval> | undefined;
let follow: ReturnType<typeof setInterval> | undefined;

/** Starts the checks: once soon after the start, then every six hours, unless the person turned them off. */
export function startAutoCheck(): () => void {
  void json<UpdateInfo>('GET', 'info').then((r) => (upd.info = r.data)).catch(() => undefined);
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
