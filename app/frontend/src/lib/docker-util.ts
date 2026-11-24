import { ApiError, type DockerContainer, type DockerImage } from './api';
import { fmtBytes as formatBytes } from './status';

export type Translate = (key: string, vars?: Record<string, string | number>) => string;
export type Tone = 'good' | 'warn' | 'muted' | 'crit';
export type LineTone = 'line' | 'info' | 'error';
export interface StreamLine {
  text: string;
  tone: LineTone;
}
export type StreamState = 'connecting' | 'live' | 'ended' | 'failed';

/** Keep at most this many lines in a log or pull view. */
export const LINE_LIMIT = 2000;

/** Colour of a container state badge. */
export function stateTone(state: string): Tone {
  switch (state) {
    case 'running':
      return 'good';
    case 'restarting':
      return 'warn';
    case 'dead':
      return 'crit';
    default:
      return 'muted';
  }
}

export function isRunning(container: DockerContainer): boolean {
  return container.state === 'running';
}

export function shortId(id: string): string {
  return id.replace(/^sha256:/, '').slice(0, 12);
}

/** First tag, or the short id for an untagged image. */
export function imageTitle(image: DockerImage): string {
  return image.repo_tags?.[0] ?? shortId(image.id);
}

export function cpuText(container: DockerContainer): string {
  if (!isRunning(container)) return '—';
  return `${container.cpu.toFixed(1)}%`;
}

export function ramText(container: DockerContainer): string {
  if (!isRunning(container)) return '—';
  const used = formatBytes(container.mem_used);
  return container.mem_limit > 0 ? `${used} / ${formatBytes(container.mem_limit)}` : used;
}

/** 503 means the server is offline, 501 means the agent module is disabled. */
export function unavailable(error: unknown): 'offline' | 'disabled' | null {
  if (error instanceof ApiError) {
    if (error.status === 503) return 'offline';
    if (error.status === 501) return 'disabled';
  }
  return null;
}

/** Human-readable text for a failed call; prefers the server's own message (e.g. 409 image in use). */
export function errorText(error: unknown, tr: Translate): string {
  if (error instanceof ApiError) {
    if (error.status === 0) return tr('common.networkError');
    if (error.status === 503) return tr('docker.offline');
    if (error.status === 501) return tr('docker.disabled');
    if (error.code) return error.code;
  }
  return tr('common.genericError');
}

/** Splits a byte stream into complete UTF-8 lines and keeps the unfinished tail. */
export class LineSplitter {
  private readonly decoder = new TextDecoder('utf-8');
  private partial = '';

  push(buffer: ArrayBuffer): string[] {
    const text = this.decoder.decode(new Uint8Array(buffer), { stream: true });
    const parts = (this.partial + text).split('\n');
    this.partial = parts.pop() ?? '';
    return parts.map((line) => line.replace(/\r$/, ''));
  }

  flush(): string[] {
    const rest = this.partial + this.decoder.decode();
    this.partial = '';
    return rest ? [rest] : [];
  }
}

/** Text frames are JSON control messages: {closed: reason} or {error: text}. */
function parseControl(text: string): { closed?: string; error?: string } | null {
  try {
    const value: unknown = JSON.parse(text);
    return value && typeof value === 'object' ? (value as { closed?: string; error?: string }) : null;
  } catch {
    return null;
  }
}

export interface StreamSink {
  open(): void;
  lines(lines: StreamLine[]): void;
  /** Called once when the socket ends; `failed` is true after an error frame or a socket error. */
  end(failed: boolean): void;
}

/** Wires a docker stream socket to a sink. Returns a function that detaches and closes the socket. */
export function attachStream(ws: WebSocket, sink: StreamSink): () => void {
  const split = new LineSplitter();
  let failed = false;
  let done = false;

  const finish = () => {
    if (done) return;
    done = true;
    const rest = split.flush();
    if (rest.length) sink.lines(rest.map((text) => ({ text, tone: 'line' as const })));
    sink.end(failed);
  };

  ws.onopen = () => sink.open();
  ws.onmessage = (event: MessageEvent) => {
    if (typeof event.data === 'string') {
      const control = parseControl(event.data);
      if (control?.error) {
        failed = true;
        sink.lines([{ text: control.error, tone: 'error' }]);
      } else if (control?.closed) {
        sink.lines([{ text: control.closed, tone: 'info' }]);
      }
      return;
    }
    if (event.data instanceof ArrayBuffer) {
      sink.lines(split.push(event.data).map((text) => ({ text, tone: 'line' as const })));
    }
  };
  ws.onerror = () => {
    failed = true;
  };
  ws.onclose = finish;

  return () => {
    done = true;
    ws.onopen = null;
    ws.onmessage = null;
    ws.onerror = null;
    ws.onclose = null;
    if (ws.readyState <= WebSocket.OPEN) ws.close();
  };
}

/** Collects lines and hands them over at most once per animation frame. */
export function createBatcher(apply: (lines: StreamLine[]) => void) {
  let queue: StreamLine[] = [];
  let frame = 0;

  const run = () => {
    frame = 0;
    const batch = queue;
    queue = [];
    if (batch.length) apply(batch);
  };

  return {
    push(lines: StreamLine[]) {
      if (lines.length === 0) return;
      queue.push(...lines);
      if (!frame) frame = requestAnimationFrame(run);
    },
    flushNow() {
      if (frame) cancelAnimationFrame(frame);
      frame = 0;
      run();
    },
    cancel() {
      if (frame) cancelAnimationFrame(frame);
      frame = 0;
      queue = [];
    },
  };
}

export interface ContainerGroup {
  /** Compose project name, or '' for the containers that do not belong to a project. */
  key: string;
  items: DockerContainer[];
  running: number;
  /** Containers with a newer image. */
  updatable: number;
  /** Directory and file of the compose project. */
  file: string;
}

/** Compose projects by name, the containers without a project last. */
export function groupContainers(list: DockerContainer[]): ContainerGroup[] {
  const map = new Map<string, ContainerGroup>();
  for (const c of list) {
    const key = c.compose ? (c.project ?? '') : '';
    let g = map.get(key);
    if (!g) {
      g = { key, items: [], running: 0, updatable: 0, file: '' };
      map.set(key, g);
    }
    g.items.push(c);
    if (isRunning(c)) g.running++;
    if (c.upd === 'new' && !c.ignored) g.updatable++;
    if (!g.file && key) g.file = c.config_file || c.workdir || '';
  }
  return [...map.values()].sort((a, b) => (a.key === '' ? 1 : b.key === '' ? -1 : a.key.localeCompare(b.key)));
}
