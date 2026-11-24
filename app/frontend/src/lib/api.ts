// The API of the Linager server (the same one the web panel uses), reached through the proxy of the application.
// Paths are relative: the page lives under a random token path.

export interface Session {
  csrf: string;
  login: string;
  role?: 'admin' | 'operator' | 'auditor';
  totp_enabled?: boolean;
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

export interface Group {
  id: number;
  name: string;
}

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
export interface DiskState { filesystems: DiskFS[] }

const enc = encodeURIComponent;
export const serverApi = {
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
  reboot: (id: string) => request<{ ok: boolean }>('POST', `api/servers/${enc(id)}/reboot`, { confirm: true }),
  deleteServer: (id: string, name: string, uninstall: boolean) => request<void>('DELETE', `api/servers/${enc(id)}`, { name, uninstall }),
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
