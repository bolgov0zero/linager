// What only the application can do. The calls go to the local server of the application (internal/local), under the
// same random path as the page itself, so they are relative.

export interface ProfileView {
  id: string;
  name: string;
  address: string;
  fingerprint: string;
  login: string;
  /** A password is saved in the keychain. */
  saved: boolean;
}

export interface AppState {
  version: string;
  connected: boolean;
  profile?: ProfileView;
  profiles: ProfileView[];
  /** The page runs inside the application window (false: in a browser during development). */
  native: boolean;
}

export class NativeError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(code);
  }
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`app/${path}`, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  const data = text ? (JSON.parse(text) as Record<string, unknown>) : {};
  if (!res.ok) throw new NativeError(res.status, typeof data.error === 'string' ? data.error : 'error');
  return data as T;
}

export const native = {
  state: () => call<AppState>('GET', 'state'),
  probe: (address: string) => call<{ fingerprint: string }>('POST', 'probe', { address }),
  connect: (address: string, fingerprint: string, name = '') => call<AppState>('POST', 'connect', { address, fingerprint, name }),
  use: (id: string) => call<AppState>('POST', 'use', { id }),
  disconnect: () => call<AppState>('POST', 'disconnect', {}),
  forget: (id: string) => call<AppState>('POST', 'forget', { id }),
  rememberLogin: (login: string) => call<{ ok: boolean }>('POST', 'login', { login }),
  secret: (profile: string) => call<{ password: string }>('GET', `secret?profile=${encodeURIComponent(profile)}`),
  saveSecret: (profile: string, password: string) => call<{ ok: boolean }>('PUT', 'secret', { profile, password }),
  forgetSecret: (profile: string) => call<{ ok: boolean }>('DELETE', `secret?profile=${encodeURIComponent(profile)}`),
  /** Opens a native window; in a browser (development) the same page opens in a new tab. */
  openWindow: async (kind: 'terminal', id: string, title: string, native: boolean): Promise<void> => {
    if (native) {
      await call<{ ok: boolean }>('POST', 'window', { kind, id, title });
      return;
    }
    window.open(`#/w/${kind}/${encodeURIComponent(id)}?title=${encodeURIComponent(title)}`, '_blank', 'popup,width=900,height=560');
  },
};
