// The API of the Linager server, reached through the proxy of the application.
// Paths are relative: the page lives under a random token path.

export interface Session {
  csrf: string;
  login: string;
  role?: 'admin' | 'operator' | 'auditor';
  /** For an operator: the modules that were given (an administrator has all, an auditor none). */
  modules?: string[];
  totp_enabled?: boolean;
  recovery_left?: number;
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
  /** A Linux agent with a graphical console the Assistant can show. */
  gui?: boolean;
  /** A Windows agent: the remote desktop (RDP) is off or the edition has none. */
  desktop_off?: boolean;
  image_updates?: number;
  mem_total?: number;
  mem_used?: number;
  swap_used?: number;
  swap_total?: number;
  load?: [number, number, number];
  boot_time?: number;
  security_updates?: number;
  disks?: Disk[];
  net?: NetIface[];
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
  /** The tags of the server with their colours (an older server does not send them). */
  tag_items?: TagItem[];
}

export interface SystemInfo {
  hostname: string;
  os: string;
  kernel: string;
  arch: string;
  cpu_model: string;
  cpu_cores: number;
  mem_total: number;
  agent_version: string;
}

export interface NetIface {
  iface: string;
  up: boolean;
  ip: string;
  mac: string;
  speed_mbps: number;
  rx_bps: number;
  tx_bps: number;
  rx_total: number;
  tx_total: number;
}

export interface Disk {
  mount: string;
  device: string;
  fs: string;
  total: number;
  used: number;
  pct: number;
}

export type Role = 'admin' | 'operator' | 'auditor';
export interface UserAccess { servers: string[]; groups: number[]; tags: number[]; modules: string[] }
export interface UserRow {
  id: number; login: string; role: string; totp_enabled: boolean; recovery_left: number;
  created_at: string; current: boolean;
  /** Only for an operator or an auditor. */
  access?: UserAccess;
}

/** Access restriction of the application port (Settings, "Application access"). */
export interface AccessRule { cidr: string; note: string }
export type AccessMode = 'off' | 'white' | 'black';
export interface AccessProblem { code: string; ip?: string; reason?: string; host?: string }
/** The restriction of the application port: its own lists. */
export interface AppAccessConfig { mode: AccessMode; white: AccessRule[]; black: AccessRule[] }
/** What a host name of a list points to now (checked_at: unix time of the last good lookup, 0 when there was none). */
export interface AccessHost { ips: string[]; checked_at: number; failing: boolean; expired: boolean; error?: string }
export interface AppAccessState { config: AppAccessConfig; you?: { ip: string; allowed: boolean; reason: string }; denied_24h: number; hosts: Record<string, AccessHost> }
export interface AppAccessCheck { problems: AccessProblem[]; you?: { ip: string; allowed: boolean; reason: string }; test?: { ip: string; allowed: boolean; reason: string }; hosts: Record<string, AccessHost> }

export interface AccessLogRow {
  id: number; ts: number; client_ip: string; remote_addr: string;
  event: 'page' | 'login.ok' | 'login.fail' | 'denied'; login: string; path: string; user_agent: string; reason: string;
}

/** An agent in the connection settings; `supported` tells whether it follows a change of the agent port. */
export interface ConnAgent { id: string; name: string; version: string; online: boolean; supported: boolean }
/** The agent port is being changed: agents move to the new port by themselves while the old one stays open. */
export interface ConnMove { from: number; to: number; moved: number; total: number; pending: ConnAgent[] }
/** Settings, "Connection": the port of the agents and the port of the desktop application. */
export interface ConnectionState {
  agent_port: number;
  /** 0: the application listener is off. */
  app_port: number;
  host: string;
  fingerprint: string;
  /** The first agent version that follows a change of the agent port. */
  min_agent: string;
  app_up: boolean;
  move: ConnMove | null;
  /** Online agents that cannot follow: while there are any, the agent port cannot be changed. */
  blockers: ConnAgent[];
}
export interface AuditItem {
  id: number; ts: number; actor: string; action: string; target_type: string; target_name: string;
  result: string; client_ip: string; remote_addr: string; details: Record<string, unknown> | null;
}
export interface AuditPage { items: AuditItem[]; next_before: number }

// ---- updates of the server and the agents ----
export interface UpdateStep {
  id: 'verify' | 'backup' | 'stop' | 'replace' | 'migrate' | 'start'; state: 'pending' | 'running' | 'done' | 'failed';
  /** Unix milliseconds; the panel often learns about the steps only after they are over, so it shows how long each took. */
  started_ms?: number; finished_ms?: number;
}
export interface ServerUpdateRun {
  run_id: string; state: 'idle' | 'running' | 'done' | 'failed' | 'rolled_back';
  steps: UpdateStep[]; from: string; to: string; error?: string; started_at: number; finished_at: number;
}
export interface AgentUpdateRow {
  id: string; name: string; os: string; address: string; version: string;
  status: 'current' | 'available' | 'unsupported' | 'updating' | 'queued' | 'rollback' | 'failed';
  online: boolean; target?: string;
  step?: number; steps_total: number;
  step_id?: 'download' | 'verify' | 'backup' | 'replace' | 'restart' | 'offline' | 'reconnect' | 'confirm';
  message?: string;
  /** A Windows agent: a newer version is out, and it is installed with its MSI package, not from the panel. */
  manual?: boolean;
  /** The stages of the run that is going on or ended badly, and the server clock they are measured by. */
  steps?: AgentUpdateStep[]; now_ms?: number;
  /** Unix seconds: when the agent restores its old binary if the new one has not confirmed itself. */
  rollback_at?: number;
}
export interface AgentUpdateStep {
  id: NonNullable<AgentUpdateRow['step_id']>; state: 'pending' | 'running' | 'done' | 'failed';
  /** Unix milliseconds on the server clock. */
  started_ms?: number; finished_ms?: number;
  detail?: string; bytes?: number; size?: number;
}
export interface UpdateHistoryRow { id: number; ts: number; kind: 'server' | 'agents'; from: string; to: string; status: 'success' | 'failed' | 'rolled_back'; detail: string }
export type RolloutMode = 'all' | 'one_by_one' | 'first_then_rest';
export interface UpdatesInfo {
  channel: 'stable' | 'beta';
  server: {
    current: string; available: string | null; checked_at: number; check_error: string;
    /** Release notes of the latest known release (bullet lines) and its version. */
    notes: string[]; notes_version?: string | null; db_migrations: { applied: number; shipped: number; pending: number };
    last_backup: number; source: string; can_update: boolean; update_run: ServerUpdateRun;
  };
  cache: { version: string; size: number; downloaded_at: number; verified: boolean; downloaded: boolean }[];
  /** Cached releases that "delete old" would remove (all but the newest and the running version). */
  cache_prunable?: number;
  /** Present in the answer of the prune call. */
  pruned?: { count: number; bytes: number };
  agents: AgentUpdateRow[];
  rollout: { state: 'running' | 'done' | 'stopped'; mode: RolloutMode; stop_on_error: boolean; version: string } | null;
  history: UpdateHistoryRow[];
  min_agent: string; agents_target: string; latest_known: string | null;
}

export interface Enrollment {
  id: string;
  hostname: string;
  address: string;
  remote_addr: string;
  os: string;
  agent_version: string;
  key_fingerprint: string;
  verify_code: string;
  expires_at: string;
  /** The label of the install link the agent came by; absent for an agent installed by hand. */
  via?: string;
  /** True when the administrator must type the key shown on the client; false for a request that came by a link. */
  needs_key: boolean;
}

/** An install link ("install by a link" of the Add server dialog). The token itself is never listed. */
export interface JoinLink {
  id: number;
  label: string;
  created_by: string;
  /** Unix seconds. */
  created_at: number;
  expires_at: number;
  /** 0 means unlimited. */
  max_uses: number;
  uses: number;
  auto_approve: boolean;
  networks: string[];
  group_id: number | null;
  group_name?: string;
  used: { host: string; at: number }[];
}
export interface JoinLinkInput { ttl_seconds: number; max_uses: number; auto_approve: boolean; networks: string[]; group_id: number | null }
export interface JoinCommand { kind: 'external' | 'local'; platform?: 'linux' | 'windows'; host: string; command: string }
/** The answer of the creation: the token and the commands exist only in this answer. */
export interface JoinLinkCreated extends JoinLink { token: string; commands: JoinCommand[] }

// ---- install over SSH ----
export type SSHPhase = 'checking' | 'ready' | 'check_failed' | 'installing' | 'done' | 'install_failed' | 'closed';
export interface SSHStep {
  id: string; state: 'pending' | 'running' | 'done' | 'warn' | 'failed';
  /** Unix milliseconds on the server clock. */
  started_ms?: number; finished_ms?: number;
  detail?: string; code?: string; bytes?: number; size?: number;
}
export interface SSHGate { kind: 'hostkey' | 'sudo' | 'address' | 'reinstall'; data?: Record<string, unknown> }
export interface SSHState {
  id: string; phase: SSHPhase; host: string; port: number; login: string;
  steps: SSHStep[]; gate?: SSHGate; error?: string;
  /** host:port the new host will use to reach this server. */
  server?: string;
  mode?: 'root' | 'nopass' | 'password';
  log: string[]; log_total: number;
  new_id?: string; new_name?: string;
  /** The server clock when the answer was made. */
  now_ms: number;
}
export interface SSHStart {
  host: string; port: number; login: string; auth: 'password' | 'key';
  password: string; key: string; passphrase: string; group_id: number | null;
}
export interface SSHAnswer { gate: string; accept: boolean; password?: string; address?: string }

export interface AvailableServer { id: string; name: string; address: string; os: string }
/** What a client needs to connect: agent port, agent-port certificate fingerprint, names/IPs of that certificate. */
export interface ConnectInfo { port: string; fingerprint: string; hosts: string[] }

export interface TotpSetup { secret: string; uri: string }

/** A tag as a server shows it: the colour is a key of the palette (see tag-colors.ts). */
export interface TagItem { id: number; name: string; color: string }
/** A tag of the registry with how many servers carry it and how many users were given access by it. */
export interface TagRow extends TagItem { servers: number; users: number }

/** A folder of servers (the server calls it a group). parent_id is null for a folder of the top level. */
export interface Group {
  id: number;
  name: string;
  parent_id: number | null;
}

/** What a change of the folders does to the access of one user: the servers they stop and start to see (the first names, and how many in all). */
export interface AccessChange { user_id: number; login: string; role: string; lose: string[]; gain: string[]; lose_n: number; gain_n: number }
export interface FolderDeletePlan { ok: boolean; dry_run: boolean; name: string; subfolders: number; servers: number; grants: string[]; impact: AccessChange[] }

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code?: string,
    /** The whole JSON answer (a 409 carries warnings in it). */
    readonly body?: unknown,
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
    let body: unknown;
    try {
      const d = (await res.json()) as { error?: unknown } | null;
      body = d;
      if (d && typeof d.error === 'string') code = d.error;
    } catch {
      // no JSON body
    }
    throw new ApiError(res.status, code, body);
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
  /** Every server the user may see (the dashboard shows only what was left on it): id, name, address, os, folder. */
  serversAll: () => request<{ id: string; name: string; address: string; os: string; group_id: number | null }[]>('GET', 'api/servers'),
  /** With dryRun nothing is written: the answer only lists whose access the change would touch. */
  serverSetGroup: (serverId: string, groupId: number | null, dryRun = false) => request<{ ok: boolean; impact: AccessChange[] }>('PUT', `api/servers/${encodeURIComponent(serverId)}/group`, { group_id: groupId, dry_run: dryRun }),
  dashboardRemove: (serverId: string) => request<void>('DELETE', `api/dashboard/items/${encodeURIComponent(serverId)}`),
  dashboardOrder: (serverIds: string[]) => request<void>('PUT', 'api/dashboard/order', { server_ids: serverIds }),
  groupCreate: (name: string, parentId: number | null = null) => request<Group>('POST', 'api/groups', { name, parent_id: parentId }),
  groupRename: (id: number, name: string) => request<void>('PUT', `api/groups/${id}`, { name }),
  /** Puts a folder under parentId (null = the top level), before beforeId (null = last). With dryRun it only says whose access it touches. */
  groupMove: (id: number, parentId: number | null, beforeId: number | null, dryRun = false) =>
    request<{ ok: boolean; dry_run: boolean; impact: AccessChange[] }>('PUT', `api/groups/${id}/move`, { parent_id: parentId, before_id: beforeId, dry_run: dryRun }),
  /** What is in the folder moves up one level. With dryRun nothing is deleted. */
  groupDelete: (id: number, dryRun = false) => request<FolderDeletePlan>('DELETE', `api/groups/${id}${dryRun ? '?dry_run=1' : ''}`),
  /** The tag registry (administrators). */
  tags: () => request<{ tags: TagRow[]; colors: string[]; max: number }>('GET', 'api/tags'),
  tagCreate: (name: string, color: string) => request<TagRow>('POST', 'api/tags', { name, color }),
  tagUpdate: (id: number, change: { name?: string; color?: string }) => request<{ ok: boolean }>('PUT', `api/tags/${id}`, change),
  /** Takes the tag off every server and out of every grant; the answer says how many there were. */
  tagDelete: (id: number) => request<{ ok: boolean; servers: number; users: number }>('DELETE', `api/tags/${id}`),

  // the person's own account
  totpSetup: () => request<TotpSetup>('POST', 'api/me/totp/setup'),
  /** Returns the recovery codes: they are shown once. */
  totpEnable: (code: string) => request<{ ok: boolean; recovery_codes: string[] }>('POST', 'api/me/totp/enable', { code }),
  totpDisable: (password: string) => request<void>('POST', 'api/me/totp/disable', { password }),
  changePassword: (current: string, password: string) => request<{ ok: boolean }>('POST', 'api/me/password', { current, password }),
  recoveryRegenerate: (password: string) => request<{ ok: boolean; recovery_codes: string[] }>('POST', 'api/me/recovery-codes', { password }),

  users: () => request<{ users: UserRow[] }>('GET', 'api/users'),
  userCreate: (login: string, password: string, role: Role = 'admin', access?: UserAccess) => request<{ ok: boolean; id: number }>('POST', 'api/users', { login, password, role, ...(access ?? {}) }),
  userAccess: (id: number, role: Role, access: UserAccess) => request<{ ok: boolean }>('PUT', `api/users/${id}/access`, { role, ...access }),
  userDelete: (id: number) => request<{ ok: boolean }>('DELETE', `api/users/${id}`, { confirm: true }),
  userSetPassword: (id: number, password: string) => request<{ ok: boolean }>('POST', `api/users/${id}/password`, { password, confirm: true }),
  userReset2fa: (id: number) => request<{ ok: boolean }>('POST', `api/users/${id}/2fa/reset`, { confirm: true }),

  appAccess: () => request<AppAccessState>('GET', 'api/app-access'),
  appAccessSave: (config: AppAccessConfig) => request<AppAccessState>('PUT', 'api/app-access', config),
  appAccessCheck: (config: AppAccessConfig, testIp?: string) => request<AppAccessCheck>('POST', 'api/app-access/check', { config, test_ip: testIp ?? '' }),
  accessResolve: (host: string) => request<{ host: string; status: AccessHost }>('POST', 'api/app-access/resolve', { host }),
  appAccessLog: (filter: 'all' | 'denied' | 'login' | 'fail', before = 0) => request<{ rows: AccessLogRow[] }>('GET', `api/app-access/log?filter=${filter}&limit=50${before ? `&before=${before}` : ''}`),

  connection: () => request<ConnectionState>('GET', 'api/connection'),
  connectionSave: (agentPort: number, appPort: number) => request<ConnectionState>('PUT', 'api/connection', { agent_port: agentPort, app_port: appPort }),
  connectionCloseOld: () => request<ConnectionState>('POST', 'api/connection/close-old'),
  audit: (params: { before?: number; limit?: number; action?: string; actor?: string; result?: string; q?: string } = {}) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '' && v !== 0) q.set(k, String(v));
    return request<AuditPage>('GET', `api/audit?${q.toString()}`);
  },

  updates: () => request<UpdatesInfo>('GET', 'api/updates'),
  updatesCheck: () => request<UpdatesInfo>('POST', 'api/updates/check'),
  updatesChannel: (channel: 'stable' | 'beta') => request<UpdatesInfo>('PUT', 'api/updates/channel', { channel }),
  updatesCachePrune: () => request<UpdatesInfo>('POST', 'api/updates/cache/prune', { confirm: true }),
  /** Downloads and verifies a release into the cache of the server ahead of time. */
  updatesPrefetch: (version: string) => request<UpdatesInfo>('POST', 'api/updates/cache', { version }),
  /** Starts the server update (backup, replace, migrate, restart); the server is unreachable for a short time. */
  updatesServer: (version: string) => request<{ ok: boolean }>('POST', 'api/updates/server', { version, confirm: true }),
  updatesAgents: (serverIds: string[] | 'all', mode: RolloutMode, stopOnError: boolean) =>
    request<UpdatesInfo>('POST', 'api/updates/agents', serverIds === 'all' ? { all: true, mode, stop_on_error: stopOnError } : { server_ids: serverIds, mode, stop_on_error: stopOnError }),

  enrollments: () => request<Enrollment[]>('GET', 'api/enrollments'),
  approveEnrollment: (id: string, key: string) => request<void>('POST', `api/enrollments/${encodeURIComponent(id)}/approve`, { key }),
  rejectEnrollment: (id: string) => request<void>('POST', `api/enrollments/${encodeURIComponent(id)}/reject`),
  joinLinks: () => request<JoinLink[]>('GET', 'api/join-tokens'),
  joinLinkCreate: (input: JoinLinkInput) => request<JoinLinkCreated>('POST', 'api/join-tokens', input),
  joinLinkRevoke: (id: number) => request<void>('DELETE', `api/join-tokens/${id}`),
  sshStart: (input: SSHStart) => request<{ id: string }>('POST', 'api/ssh-install', input),
  sshState: (id: string) => request<SSHState>('GET', `api/ssh-install/${encodeURIComponent(id)}`),
  sshAnswer: (id: string, answer: SSHAnswer) => request<SSHState>('POST', `api/ssh-install/${encodeURIComponent(id)}/answer`, answer),
  sshInstall: (id: string) => request<SSHState>('POST', `api/ssh-install/${encodeURIComponent(id)}/install`, {}),
  sshClose: (id: string) => request<void>('DELETE', `api/ssh-install/${encodeURIComponent(id)}`),
  connectInfo: () => request<ConnectInfo>('GET', 'api/connect'),
  availableServers: () => request<AvailableServer[]>('GET', 'api/dashboard/available'),
  dashboardAdd: (serverId: string) => request<void>('POST', 'api/dashboard/items', { server_id: serverId }),

  snippetsList: () => request<{ snippets: Snippet[] }>('GET', 'api/snippets'),
  snippetCreate: (name: string, command: string) => request<Snippet>('POST', 'api/snippets', { name, command }),
  snippetUpdate: (id: number, name: string, command: string) => request<{ ok: boolean }>('PUT', `api/snippets/${id}`, { name, command }),
  snippetDelete: (id: number) => request<{ ok: boolean }>('DELETE', `api/snippets/${id}`),
  snippetsOrder: (ids: number[]) => request<{ ok: boolean }>('PUT', 'api/snippets/order', { ids }),
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
export function openTerminal(id: string, cols: number, rows: number, user = ''): WebSocket {
  const url = new URL(`api/servers/${encodeURIComponent(id)}/stream`, location.href);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  url.searchParams.set('kind', 'terminal');
  url.searchParams.set('cols', String(cols));
  url.searchParams.set('rows', String(rows));
  if (user) url.searchParams.set('user', user);
  const ws = new WebSocket(url);
  ws.binaryType = 'arraybuffer';
  return ws;
}

export interface ServerDetail {
  /** The enabled agent modules; empty means unknown (an old agent). */
  modules: string[];
  id: string;
  name: string;
  hostname: string;
  address: string;
  os: string;
  agent_version: string;
  online: boolean;
  last_seen_at: string | null;
  enrolled_at: string;
  system: SystemInfo | null;
  sample: Sample | null;
  spark: number[];
  /** The tags of the server (kept by the Linager server): the names, and the same tags with their ids and colours. */
  tags?: string[];
  tag_items?: TagItem[];
  note?: string;
  note_updated_at?: number;
  note_updated_by?: string;
  recent: { ts: number; cpu: number; mem: number; rx: number; tx: number }[];
}

export type Period = '1h' | '24h' | '7d' | '30d' | '90d' | '180d' | '360d';
export type MetricKey = 'cpu' | 'mem' | 'disk' | 'rx' | 'tx';
export interface HistoryPoint {
  ts: number;
  cpu_avg: number; cpu_max: number;
  mem_avg: number; mem_max: number;
  disk_avg: number; disk_max: number;
  rx_avg: number; rx_max: number;
  tx_avg: number; tx_max: number;
}
export interface MetricStats { min: number; avg: number; p95: number; max: number }
export interface HistoryEvent { ts: number; kind: 'reboot' | 'service_failed' | 'offline' | 'online'; message: string }
export interface History {
  period: Period;
  tier: string;
  step_seconds: number;
  from: number;
  to: number;
  points: HistoryPoint[];
  stats: Record<MetricKey, MetricStats>;
  events: HistoryEvent[];
}

export interface ServiceUnit {
  name: string;
  description: string;
  active: string;
  sub: string;
  enabled: string;
}
export type ServiceAction = 'start' | 'stop' | 'restart' | 'enable' | 'disable';

export interface ProcessInfo { pid: number; user: string; cpu: number; mem_pct: number; rss: number; state: string; cmd: string }
export interface ProcessList { processes: ProcessInfo[]; total: number }

export interface AptPackage { name: string; current: string; candidate: string; security: boolean; reboot: boolean; ignored?: boolean }
export interface AptList { packages: AptPackage[]; reboot_required: boolean; last_check: number; busy?: boolean }

export interface DiskFS {
  mount: string; device: string; fs: string; total: number; used: number; avail: number;
  inodes_total: number; inodes_used: number; pct: number;
  per_day: number; days_left: number; history_days: number; trend: number[];
}
export interface DiskCleanItem { kind: 'journal' | 'apt' | 'docker-images' | 'docker-build' | 'kernels' | 'win-temp' | 'win-update-cache' | 'win-recycle' | 'win-delivery' | 'win-dumps' | 'win-old'; size: number; frees: number; count?: number; items?: string[]; keep?: string }
export interface DiskState { filesystems: DiskFS[]; cleanup: DiskCleanItem[]; docker: boolean }
export interface DiskNode { name: string; size: number; children?: DiskNode[] }
export interface DiskScan { scanned_at: number; mount: string; partial: boolean; tree: DiskNode; categories: { id: string; size: number }[] }

const enc = encodeURIComponent;
export const serverApi = {
  setTags: (id: string, tags: string[]) => request<{ tags: string[]; tag_items: TagItem[] }>('PUT', `api/servers/${enc(id)}/tags`, { tags }),
  detail: (id: string) => request<ServerDetail>('GET', `api/servers/${enc(id)}`),
  history: (id: string, period: Period) => request<History>('GET', `api/servers/${enc(id)}/metrics?period=${period}`),
  services: (id: string) => request<{ services: ServiceUnit[] }>('GET', `api/servers/${enc(id)}/services`),
  /** Rejects with status 409 and code `dangerous` when the action needs confirm: true. */
  serviceAction: (id: string, unit: string, action: ServiceAction, confirm: boolean) =>
    request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/services/${enc(unit)}/${action}`, { confirm }),
  processes: (id: string, sort: 'cpu' | 'mem' | 'name', n = 30) => request<ProcessList>('GET', `api/servers/${enc(id)}/processes?sort=${sort}&n=${n}`),
  killProcess: (id: string, pid: number, signal: 'TERM' | 'KILL') =>
    request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/processes/${pid}/kill`, { signal, confirm: true }),
  apt: (id: string) => request<AptList>('GET', `api/servers/${enc(id)}/apt`),
  disk: (id: string) => request<DiskState>('GET', `api/servers/${enc(id)}/disk`),
  diskScan: (id: string, mount: string, force: boolean) => request<DiskScan>('POST', `api/servers/${enc(id)}/disk/scan`, { mount, force }),
  diskClean: (id: string, kind: DiskCleanItem['kind']) => request<{ ok: boolean; freed: number; output: string }>('POST', `api/servers/${enc(id)}/disk/clean`, { kind, confirm: true }),
  reboot: (id: string) => request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/reboot`, { confirm: true }),
  deleteServer: (id: string, name: string, uninstall: boolean) => request<void>('DELETE', `api/servers/${enc(id)}`, { name, uninstall }),
  servicesIgnored: (id: string) => request<{ ignored: string[] }>('GET', `api/servers/${encodeURIComponent(id)}/services-ignored`),
  serviceIgnore: (id: string, unit: string, ignore: boolean) => request<{ ok: boolean }>('PUT', `api/servers/${encodeURIComponent(id)}/services-ignored`, { unit, ignore }),
  aptIgnore: (id: string, name: string, ignore: boolean) => request<{ ok: boolean }>('PUT', `api/servers/${encodeURIComponent(id)}/apt-ignore`, { name, ignore }),
  noteSet: (id: string, note: string) => request<{ note: string }>('PUT', `api/servers/${enc(id)}/note`, { note }),
};

/** The log stream of a server (journal): text lines over a WebSocket. */
/** The apt stream: a check (`update`), or an install of everything, of the security fixes or of the chosen packages. */
export function openApt(id: string, mode: 'update' | 'upgrade' | 'security' | 'packages', names: string[] = []): WebSocket {
  const url = new URL(`api/servers/${enc(id)}/stream`, location.href);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  url.searchParams.set('kind', 'apt');
  url.searchParams.set('mode', mode);
  if (mode === 'packages') url.searchParams.set('names', names.join(','));
  const ws = new WebSocket(url);
  ws.binaryType = 'arraybuffer';
  return ws;
}

/** A stream of the server (a job with output: an install, a pull). Text frames are JSON `{closed}` or `{error}`; binary ones are text. */
export function openStream(id: string, kind: string, params: Record<string, string | number> = {}): WebSocket {
  const url = new URL(`api/servers/${enc(id)}/stream`, location.href);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  url.searchParams.set('kind', kind);
  for (const [k, v] of Object.entries(params)) url.searchParams.set(k, String(v));
  const ws = new WebSocket(url);
  ws.binaryType = 'arraybuffer';
  return ws;
}

export function openLogs(id: string, params: { unit?: string; priority?: number; kernel?: boolean; lines?: number }): WebSocket {
  const url = new URL(`api/servers/${enc(id)}/stream`, location.href);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  url.searchParams.set('kind', 'logs');
  url.searchParams.set('lines', String(params.lines ?? 200));
  if (params.unit) url.searchParams.set('unit', params.unit);
  if (params.priority !== undefined) url.searchParams.set('priority', String(params.priority));
  if (params.kernel) url.searchParams.set('kernel', '1');
  const ws = new WebSocket(url);
  ws.binaryType = 'arraybuffer';
  return ws;
}

export interface Snippet { id: number; name: string; command: string; position: number }

export interface FileStat {
  name: string; path: string; type: 'file' | 'dir' | 'link' | 'other'; size: number;
  mode: string; mode_str: string; owner: string; group: string; uid: number; gid: number;
  mtime: number; atime: number; ctime: number; btime: number; links: number; inode: number;
  link_target?: string; mime?: string; protected: boolean;
}
export type FileConflictMode = 'fail' | 'overwrite' | 'keep';

export interface FileEntry {
  name: string;
  type: 'dir' | 'file' | 'link' | 'other';
  size: number;
  mode: string;
  mode_str: string;
  owner: string;
  group: string;
  mtime: number;
  link_target?: string;
}
export interface FileList { path: string; entries: FileEntry[] }
export interface FileContent { path: string; content: string; size: number; mtime: number; mode: string }

export interface HostUser {
  name: string; uid: number; gid: number; gecos: string; home: string; shell: string;
  groups: string[]; sudo: 'none' | 'pw' | 'nopw' | 'root'; locked: boolean; has_password: boolean; pw_changed: number;
  last_login: number; last_from: string; sessions: number; procs: number; keys: number;
  human: boolean; protected: boolean; no_shell: boolean;
}
export interface HostUserAdd {
  name: string; gecos: string; shell: string; home: string; groups: string[]; sudo: 'none' | 'pw' | 'nopw'; password: string; ssh_key: string;
}
export interface HostUserPatch { groups?: string[]; sudo?: 'none' | 'pw' | 'nopw'; shell?: string; locked?: boolean }
export interface HostUsersState { users: HostUser[]; groups: string[]; sudo_group: string; root_login: boolean; shells: string[] }

export interface SecJail { name: string; currently_failed: number; total_failed: number; currently_banned: number; total_banned: number; banned_ips: string[] }
export interface F2bBan { ip: string; jail: string; banned_at: number; unban_at: number; attempts: number }
/** Parameters of the sshd jail; times are in seconds, a ban time of -1 means forever. */
export interface F2bSettings { maxretry: number; findtime: number; bantime: number; increment: boolean; recidive: boolean; ignore: string[] }
export interface F2bEvent { time: number; event: 'ban' | 'unban'; ip: string; jail: string }
export interface F2bHistory { events: F2bEvent[]; total: number; log: string; available: boolean }
export interface SecState {
  generated_at: number; period_days: number;
  /** The address the administrator is connected from (never blocked by the panel). */
  self_ip?: string;
  ssh: {
    installed: boolean; source: 'effective' | 'files' | ''; ports: string[]; permit_root_login: string; password_auth: string;
    pubkey_auth: string; kbd_interactive: string; permit_empty_passwords: string; max_auth_tries: string; user_restrictions: boolean;
  };
  fail2ban: { installed: boolean; active: boolean; jails: SecJail[]; bans: F2bBan[]; settings?: F2bSettings; managed: boolean };
  auto_updates: boolean;
  logins: { time: number; user: string; ip: string; method: string; prior_failures: number }[];
  failures: {
    total: number; invalid_user_attempts: number; addresses: number; by_day: number[];
    top_ips: { ip: string; count: number; users: number; last: number }[];
    top_users: { user: string; count: number; invalid: boolean }[];
  };
  keys: { user: string; file: string; type: string; bits?: number; fingerprint: string; comment: string; restricted: boolean; weak: boolean; mtime: number }[];
  sessions: { id: string; user: string; ip: string; since: number }[];
  journal: { source: 'journal' | 'authlog' | 'none'; oldest: number; covers_period: boolean; truncated: boolean };
}

export interface NetInterface {
  name: string; state: 'UP' | 'DOWN'; kind: 'physical' | 'virtual' | 'loopback';
  mac: string; mtu: number; mode: 'static' | 'dhcp' | 'manual' | 'unknown';
  addresses: string[]; gateway: string; speed_mbps: number;
  rx_bytes: number; tx_bytes: number; rx_packets: number; tx_packets: number;
  rx_errors: number; tx_errors: number; rx_dropped: number; tx_dropped: number;
  primary: boolean; // carries the agent's connection
  editable: boolean;
}
export interface NetRoute { dst: string; gateway: string; dev: string; metric: number; protocol: string }
export interface NetChange { iface: string; mode: 'static' | 'dhcp'; address: string; gateway: string; mtu: number; up: boolean }
export interface NetworkDraft { changes: NetChange[] }
export interface PendingChange { id: string; kind: 'network' | 'firewall'; deadline: number /* unix seconds */ }
export interface NetplanFile { name: string; text: string; cloud_init: boolean }
export interface NetworkInfo {
  backend: 'ifupdown' | 'netplan' | 'unknown';
  editable: boolean;
  interfaces: NetInterface[];
  dns: { nameservers: string[]; search: string[] };
  dns_editable?: boolean;
  dns_backend?: 'resolved' | 'resolvconf' | 'file' | 'netplan' | 'unknown';
  routes: NetRoute[];
  config_preview: string;
  config_editable?: boolean;
  netplan_files?: NetplanFile[];
  netplan_effective?: string;
  pending: PendingChange | null;
  draft: NetworkDraft | null;
}
export interface FieldError { iface: string; field: string; message: string }
export interface ApplyWarning { code: 'primary_iface' | 'ssh_port' | 'agent_link'; message: string }

export interface FwRule { text: string; packets?: number; bytes?: number }
export interface FwChain { name: 'input' | 'forward' | 'output'; policy: 'accept' | 'drop'; rules: FwRule[] }
/** A named list of addresses (nftables set) that rules use as `ip saddr @name`. */
export interface FwSet { name: string; family: 'ipv4' | 'ipv6'; elements: string[] }
export interface FirewallDraft { sets?: FwSet[]; chains: { name: FwChain['name']; policy: FwChain['policy']; rules: { text: string }[] }[] }
/** A packet that a rule marked for logging matched (from the kernel journal). */
export interface FwLogEvent {
  time: number; chain: 'input' | 'forward' | 'output'; rule: string; action: 'accept' | 'drop' | 'reject';
  iface: string; src: string; dst: string; proto: string; sport?: number; dport?: number; len?: number; flags?: string;
}
export interface FwLog { available: boolean; events: FwLogEvent[]; total: number; truncated?: boolean }
/** One listening port with what the firewall says about it. */
export interface FwListener {
  proto: 'tcp' | 'udp'; port: number; addrs: string[]; process: string; pid: number; conns: number;
  scope: 'all' | 'local' | 'addr' | 'docker';
  firewall: 'allowed' | 'restricted' | 'blocked' | 'open' | 'policy' | 'local' | 'docker';
  risky: boolean;
}
export interface FwPorts {
  available: boolean; policy?: 'accept' | 'drop'; listeners: FwListener[];
  remotes: { addr: string; port: number; conns: number }[]; exposed?: number;
}
export interface FirewallInfo {
  available: boolean; ufw_active: boolean;
  chains: FwChain[]; sets?: FwSet[]; ruleset: string; other_tables: string[];
  pending: PendingChange | null; draft: FirewallDraft | null;
}

export interface DockerContainer {
  id: string; name: string; image: string; state: string; status: string; ports: string[];
  created: number; cpu: number; mem_used: number; mem_limit: number;
  /** The container belongs to a docker compose project (it is updated through compose). */
  compose?: boolean;
  project?: string; service?: string; workdir?: string; config_file?: string;
  /** Result of the last image check: new (a newer image exists), current, unknown. Empty if not checked. */
  upd?: 'new' | 'current' | 'unknown' | '';
  upd_local?: string; upd_remote?: string; upd_error?: string;
  /** Updates of this container are ignored: it stays in the list but is not counted in the badges. */
  ignored?: boolean;
}
export interface DockerImage { id: string; repo_tags: string[]; size: number; created: number; used: boolean }
export interface DockerInfo { available: boolean; containers: DockerContainer[]; images: DockerImage[]; checked_at?: number }
/** The editable settings of a container (the "Edit container" dialog). ip "0.0.0.0" means all addresses. */
export interface DockerEditPort { ip: string; host: number; cont: number; proto: 'tcp' | 'udp' }
export interface DockerEditMount { type: 'bind' | 'volume'; src: string; dst: string; ro: boolean }
export interface DockerEditEnv { k: string; v: string }
export interface DockerEditSpec {
  name: string; image: string;
  restart: 'no' | 'always' | 'unless-stopped' | 'on-failure'; retries: number;
  cmd: string;
  mem_mb: number; cpus: number;
  ports: DockerEditPort[]; mounts: DockerEditMount[]; env: DockerEditEnv[];
}
export interface DockerEditConfig extends DockerEditSpec {
  running: boolean;
  /** Set for a compose container: only restart, mem_mb and cpus are editable. */
  compose?: { project: string; file: string };
}
export interface DockerCheckResult { checked_at: number; new: number; current: number; unknown: number }

// ---- remote desktop (RDP through the agent) ----
export interface DesktopState {
  enabled: boolean; listening: boolean; port: number; nla: boolean; service: string; supported: boolean; edition: string;
}

/** The WebSocket of the remote desktop client (RDCleanPath over binary messages). login is only written to the audit log. */
export function desktopSocketUrl(id: string, login: string): string {
  const url = new URL(`api/servers/${enc(id)}/desktop`, location.href);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  if (login) url.searchParams.set('login', login.slice(0, 64));
  return url.toString();
}

export const desktopApi = {
  state: (id: string) => request<DesktopState>('GET', `api/servers/${enc(id)}/desktop/state`),
  enable: (id: string) => request<DesktopState>('POST', `api/servers/${enc(id)}/desktop/enable`, { confirm: true }),
  disable: (id: string) => request<DesktopState>('POST', `api/servers/${enc(id)}/desktop/disable`, { confirm: true }),
};

// ---- the Assistant (the screen of a Windows session) ----
/** A Windows session the Assistant can show; `screen` is false for a disconnected one (nothing to look at). */
export interface AssistSession { id: number; type: 'console' | 'rdp' | 'other'; user: string; state: string; client?: string; console: boolean; screen: boolean }
/** `show_cursor`: the pointer of the computer is drawn into the picture; off by default (the page then shows its own, without lag). */
export interface AssistSettings { colors: '24' | '16' | '256' | 'gray'; fps: number; mbit: number; nolimit: boolean; auto: boolean; show_cursor: boolean }
/** Whether Windows lets the agent service send Ctrl+Alt+Del (the secure attention sequence). */
export interface AssistSas { policy: number; enabled: boolean }

/** The WebSocket of the Assistant: `session` is 'console' or the number of a Windows session. Binary frames both ways. */
export function assistantSocketUrl(id: string, session: string): string {
  const url = new URL(`api/servers/${enc(id)}/assistant`, location.href);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  url.searchParams.set('session', session);
  return url.toString();
}

export const assistantApi = {
  /** 501 (ApiError) when the agent of the computer has no Assistant module. */
  sessions: (id: string) => request<{ sessions: AssistSession[]; sas?: AssistSas }>('GET', `api/servers/${enc(id)}/assistant/sessions`),
  setSas: (id: string, enable: boolean) => request<AssistSas>('POST', `api/servers/${enc(id)}/assistant/sas`, { enable, confirm: true }),
  settings: (id: string) => request<AssistSettings>('GET', `api/servers/${enc(id)}/assistant/settings`),
  /** 422 (ApiError with the text) for a value out of range; `applied` is the number of open sessions that took it. */
  saveSettings: (id: string, settings: AssistSettings) => request<{ settings: AssistSettings; applied: number }>('PUT', `api/servers/${enc(id)}/assistant/settings`, settings),
};

// ---- Windows modules: users, Windows Update, applications, network adapters ----
export interface WinUser {
  name: string; full_name: string; description: string; enabled: boolean; sid: string; builtin: boolean;
  last_logon: number; pw_last_set: number; pw_expires: number; groups: string[];
}
export interface WinSession { id: number; user: string; state: string; type: 'console' | 'rdp' | 'other' }
export interface WinUsersState { users: WinUser[]; groups: string[]; sessions: WinSession[] }
export interface WinApp { name: string; version: string; publisher: string; installed: string; size: number; scope: 'machine' | 'user'; arch: string }
export interface WinUpdate { id: string; title: string; kb: string; size: number; severity: string; security: boolean; downloaded: boolean; need_reboot: boolean }
export interface WinAdapter {
  index: number; name: string; description: string; mac: string; up: boolean; virtual: boolean; mtu: number; speed_mbps: number;
  dhcp: boolean; ipv4: string[]; gateway: string; dns: string[];
}
export interface WinNetState { adapters: WinAdapter[]; pending: { id: string; deadline: number } | null }
export interface WinNetApply { index: number; mode: 'dhcp' | 'static'; ip?: string; prefix?: number; gateway?: string; dns: string[] }

/** One call to a Windows module of the agent. The dangerous ones are sent with the confirmation the person gave. */
export function winCall<T>(id: string, module: string, action: string, params: Record<string, unknown> = {}, confirm = false): Promise<T> {
  return request<T>('POST', `api/servers/${enc(id)}/win/${module}/${action}`, confirm ? { ...params, confirm: true } : params);
}

export const serverApi2 = {
  snippets: () => request<{ snippets: Snippet[] }>('GET', 'api/snippets'),
  snippetRan: (id: number, serverId: string) => request<{ ok: boolean }>('POST', `api/snippets/${id}/ran`, { server_id: serverId }),
  files: (id: string, path: string) => request<FileList>('GET', `api/servers/${enc(id)}/files?path=${enc(path)}`),
  fileContent: (id: string, path: string) => request<FileContent>('GET', `api/servers/${enc(id)}/files/content?path=${enc(path)}`),
  fileSave: (id: string, path: string, content: string, mtime?: number) =>
    request<{ ok: boolean; mtime: number }>('PUT', `api/servers/${enc(id)}/files/content`, { path, content, mtime }),
  fileMkdir: (id: string, path: string) => request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/files/mkdir`, { path }),
  fileCreate: (id: string, path: string) => request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/files/create`, { path }),
  fileDelete: (id: string, path: string) => request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/files/delete`, { path, confirm: true }),
  fileRename: (id: string, path: string, name: string) => request<{ ok: boolean; name: string }>('POST', `api/servers/${enc(id)}/files/rename`, { path, name }),
  fileCopy: (id: string, path: string, dir: string, mode: FileConflictMode = 'fail') =>
    request<{ ok: boolean; name: string }>('POST', `api/servers/${enc(id)}/files/copy`, { path, dir, mode, confirm: mode === 'overwrite' }),
  fileMove: (id: string, path: string, dir: string, mode: FileConflictMode = 'fail') =>
    request<{ ok: boolean; name: string }>('POST', `api/servers/${enc(id)}/files/move`, { path, dir, mode, confirm: mode === 'overwrite' }),
  fileStat: (id: string, path: string) => request<FileStat>('GET', `api/servers/${enc(id)}/files/stat?path=${enc(path)}`),
  fileDu: (id: string, path: string) => request<{ size: number; files: number; dirs: number; truncated: boolean }>('GET', `api/servers/${enc(id)}/files/du?path=${enc(path)}`),
  fileChmod: (id: string, path: string, mode: string, recursive: boolean, confirm: boolean) =>
    request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/files/chmod`, { path, mode, recursive, confirm }),
  fileChown: (id: string, path: string, owner: string, group: string, recursive: boolean) =>
    request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/files/chown`, { path, owner, group, recursive, confirm: true }),
  fileDownloadUrl: (id: string, path: string) => `api/servers/${enc(id)}/files/download?path=${enc(path)}`,
  fileZipUrl: (id: string, dir: string, names: string[]) => `api/servers/${enc(id)}/files/zip?dir=${enc(dir)}${names.map((n) => `&name=${enc(n)}`).join('')}`,
  uploadFile: (id: string, path: string, file: File, onProgress?: (fraction: number) => void) =>
    new Promise<void>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open('POST', `api/servers/${enc(id)}/files/upload?path=${enc(path)}`);
      xhr.setRequestHeader('X-CSRF-Token', csrf);
      xhr.upload.onprogress = (e) => e.lengthComputable && onProgress?.(e.loaded / e.total);
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) return resolve();
        let code: string | undefined;
        try {
          code = (JSON.parse(xhr.responseText) as { error?: string }).error;
        } catch {
          // no body
        }
        reject(new ApiError(xhr.status, code));
      };
      xhr.onerror = () => reject(new ApiError(0, 'network'));
      xhr.send(file);
    }),
  hostUsers: (id: string) => request<HostUsersState>('GET', `api/servers/${enc(id)}/users`),
  hostUserAdd: (id: string, u: HostUserAdd) => request<{ ok: boolean; warnings: string[] }>('POST', `api/servers/${enc(id)}/users`, { ...u, confirm: true }),
  hostUserUpdate: (id: string, name: string, patch: HostUserPatch) => request<{ ok: boolean }>('PUT', `api/servers/${enc(id)}/users/${enc(name)}`, { ...patch, confirm: true }),
  hostUserDelete: (id: string, name: string, removeHome: boolean) => request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/users/${enc(name)}/delete`, { remove_home: removeHome, confirm: true }),
  hostUserPassword: (id: string, name: string, password: string) => request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/users/${enc(name)}/password`, { password, confirm: true }),
  hostUserKill: (id: string, name: string) => request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/users/${enc(name)}/kill`, { confirm: true }),
  securityBan: (id: string, ip: string, duration: number) => request<{ ok: boolean; timed: boolean }>('POST', `api/servers/${enc(id)}/security/ban`, { ip, duration, confirm: true }),
  securityUnban: (id: string, target: { ips: string[] } | { all: true }) => request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/security/unban`, { ...target, confirm: true }),
  securityF2bSettings: (id: string, settings: F2bSettings) => request<{ ok: boolean }>('PUT', `api/servers/${enc(id)}/security/fail2ban`, { ...settings, confirm: true }),
  securityBans: (id: string, limit = 200) => request<F2bHistory>('GET', `api/servers/${enc(id)}/security/bans?limit=${limit}`),
  securityKeyRemove: (id: string, key: { user: string; file: string; fingerprint: string }) => request<{ ok: boolean; removed: number; backup: string }>('POST', `api/servers/${enc(id)}/security/keys/remove`, { ...key, confirm: true }),
  securityTerminate: (id: string, target: { session: string } | { user: string }) =>
    request<{ ok: boolean; terminated: { id: string; user: string; ip: string }[] }>('POST', `api/servers/${enc(id)}/security/terminate`, { ...target, confirm: true }),
  security: (id: string) => request<SecState>('GET', `api/servers/${enc(id)}/security`),
  network: (id: string) => request<NetworkInfo>('GET', `api/servers/${enc(id)}/network`),
  networkValidate: (id: string, draft: NetworkDraft) => request<{ errors: FieldError[] }>('POST', `api/servers/${enc(id)}/network/validate`, draft),
  networkSaveDraft: (id: string, draft: NetworkDraft) => request<{ ok: boolean }>('PUT', `api/servers/${enc(id)}/network/draft`, draft),
  networkResetDraft: (id: string) => request<{ ok: boolean }>('DELETE', `api/servers/${enc(id)}/network/draft`),
  /** Without confirm a risky change answers 409 with warnings (ApiError.body); repeat with confirm. */
  networkApply: (id: string, confirm: boolean) => request<PendingChange>('POST', `api/servers/${enc(id)}/network/apply`, { confirm }),
  networkApplyConfig: (id: string, text: string, file?: string) => request<PendingChange>('POST', `api/servers/${enc(id)}/network/config`, { text, file, confirm: true }),
  networkApplyDns: (id: string, nameservers: string[], search: string[]) => request<PendingChange>('POST', `api/servers/${enc(id)}/network/dns`, { nameservers, search, confirm: true }),
  /** Confirms or rolls back a change with the timer (kind: network or firewall). */
  changeConfirm: (id: string, kind: 'network' | 'firewall', changeId: string) => request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/${kind}/confirm`, { id: changeId }),
  changeRollback: (id: string, kind: 'network' | 'firewall', changeId: string) => request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/${kind}/rollback`, { id: changeId }),
  networkConfirm: (id: string, changeId: string) => request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/network/confirm`, { id: changeId }),
  networkRollback: (id: string, changeId: string) => request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/network/rollback`, { id: changeId }),
  firewallLog: (id: string) => request<FwLog>('GET', `api/servers/${enc(id)}/firewall/log`),
  firewallPorts: (id: string) => request<FwPorts>('GET', `api/servers/${enc(id)}/firewall/ports`),
  firewallValidate: (id: string, draft: FirewallDraft) => request<{ ok: boolean; error?: string }>('POST', `api/servers/${enc(id)}/firewall/validate`, draft),
  firewallSaveDraft: (id: string, draft: FirewallDraft) => request<{ ok: boolean }>('PUT', `api/servers/${enc(id)}/firewall/draft`, draft),
  firewallResetDraft: (id: string) => request<{ ok: boolean }>('DELETE', `api/servers/${enc(id)}/firewall/draft`),
  firewallApply: (id: string, confirm: boolean) => request<PendingChange>('POST', `api/servers/${enc(id)}/firewall/apply`, { confirm }),
  firewall: (id: string) => request<FirewallInfo>('GET', `api/servers/${enc(id)}/firewall`),
  docker: (id: string) => request<DockerInfo>('GET', `api/servers/${enc(id)}/docker`),
  dockerContainerAction: (id: string, cid: string, action: 'start' | 'stop' | 'restart' | 'remove') =>
    request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/docker/containers/${enc(cid)}/${action}`, { confirm: action === 'remove' }),
  dockerIgnore: (id: string, name: string, ignore: boolean) => request<{ ok: boolean }>('PUT', `api/servers/${enc(id)}/docker-ignore`, { name, ignore }),
  dockerCheck: (id: string) => request<DockerCheckResult>('POST', `api/servers/${enc(id)}/docker/check`, {}),
  dockerConfig: (id: string, cid: string) => request<DockerEditConfig>('GET', `api/servers/${enc(id)}/docker/containers/${enc(cid)}/config`),
  /** Starts the change; the progress comes over the `docker-edit` stream opened with the returned token. */
  dockerEdit: (id: string, cid: string, spec: DockerEditSpec) => request<{ token: string }>('POST', `api/servers/${enc(id)}/docker/containers/${enc(cid)}/edit`, { spec, confirm: true }),
  dockerRemoveImage: (id: string, imageId: string) => request<{ ok: boolean }>('DELETE', `api/servers/${enc(id)}/docker/images/${enc(imageId)}`),
};
