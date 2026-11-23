import type { LiveEvent } from './api';

type Handler = (e: LiveEvent) => void;

const handlers = new Set<Handler>();
let source: EventSource | null = null;
let retry: ReturnType<typeof setTimeout> | undefined;

function connect(): void {
  source = new EventSource('api/live', { withCredentials: true });
  const forward = (type: LiveEvent['type']) => (ev: MessageEvent<string>) => {
    let data: Record<string, unknown> = {};
    try {
      data = ev.data ? (JSON.parse(ev.data) as Record<string, unknown>) : {};
    } catch {
      return;
    }
    const e = { type, ...data } as LiveEvent;
    handlers.forEach((h) => h(e));
  };
  source.addEventListener('metrics', forward('metrics'));
  source.addEventListener('status', forward('status'));
  source.addEventListener('dashboard', forward('dashboard'));
  source.onerror = () => {
    source?.close();
    source = null;
    if (handlers.size > 0) retry = setTimeout(connect, 3000);
  };
}

/** Subscribe to live events; the shared connection opens on first subscriber and closes after the last. Returns unsubscribe. */
export function subscribeLive(h: Handler): () => void {
  handlers.add(h);
  if (!source && retry === undefined) connect();
  return () => {
    handlers.delete(h);
    if (handlers.size === 0) {
      source?.close();
      source = null;
      if (retry) clearTimeout(retry);
      retry = undefined;
    }
  };
}
