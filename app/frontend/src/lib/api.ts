// The API of the Linager server (the same one the web panel uses), reached through the proxy of the application.
// Paths are relative: the page lives under a random token path.

export interface Session {
  csrf: string;
  login: string;
  role?: 'admin' | 'operator' | 'auditor';
  totp_enabled?: boolean;
}

export interface Disk {
  mount?: string;
  pct?: number;
}

export interface Sample {
  ts: number;
  cpu: number;
  mem_pct: number;
  disk_pct: number;
  rx_bps: number;
  tx_bps: number;
  uptime: number;
  failed_services: number;
  reboot_required: boolean;
  updates: number;
  image_updates?: number;
  disks?: Disk[];
}

export interface ServerCard {
  id: string;
  name: string;
  address: string;
  os: string;
  agent_version: string;
  online: boolean;
  last_seen_at: string | null;
  sample: Sample | null;
  /** The last CPU values, 5 seconds apart, oldest first. */
  spark: number[];
  group_id: number | null;
}

export interface Group {
  id: number;
  name: string;
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code?: string,
  ) {
    super(code ?? `HTTP ${status}`);
  }
}

let csrf = '';

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (method !== 'GET') headers['X-CSRF-Token'] = csrf;
  let res: Response;
  try {
    res = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  } catch {
    throw new ApiError(0, 'network');
  }
  if (!res.ok) {
    let code: string | undefined;
    try {
      const d = (await res.json()) as { error?: unknown } | null;
      if (d && typeof d.error === 'string') code = d.error;
    } catch {
      // no JSON body
    }
    throw new ApiError(res.status, code);
  }
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

export const api = {
  async me(): Promise<Session> {
    const s = await request<Session>('GET', 'api/me');
    csrf = s.csrf;
    return s;
  },
  async login(login: string, password: string, totp?: string): Promise<Session> {
    const s = await request<Session>('POST', 'api/login', { login, password, totp });
    csrf = s.csrf;
    return s;
  },
  async logout(): Promise<void> {
    try {
      await request<void>('POST', 'api/logout', {});
    } finally {
      csrf = '';
    }
  },
  dashboard: () => request<{ items: ServerCard[] }>('GET', 'api/dashboard'),
  groups: () => request<Group[]>('GET', 'api/groups'),
};

/** The shared stream of live events; `handler` is called for every event and the stream reconnects by itself. */
export function subscribeLive(handler: () => void): () => void {
  let source: EventSource | null = null;
  let retry: ReturnType<typeof setTimeout> | undefined;
  let stopped = false;
  const open = (): void => {
    source = new EventSource('api/live');
    for (const type of ['metrics', 'status', 'dashboard']) source.addEventListener(type, handler);
    source.onerror = () => {
      source?.close();
      source = null;
      if (!stopped) retry = setTimeout(open, 3000);
    };
  };
  open();
  return () => {
    stopped = true;
    source?.close();
    if (retry) clearTimeout(retry);
  };
}

/** The WebSocket of a terminal of a server. */
export function openTerminal(id: string, cols: number, rows: number): WebSocket {
  const url = new URL(`api/servers/${encodeURIComponent(id)}/stream`, location.href);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  url.searchParams.set('kind', 'terminal');
  url.searchParams.set('cols', String(cols));
  url.searchParams.set('rows', String(rows));
  const ws = new WebSocket(url);
  ws.binaryType = 'arraybuffer';
  return ws;
}
