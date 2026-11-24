/** The wire format of the Assistant socket: every binary message is one frame, a type byte and a payload. */

/** Server to browser. */
export const MSG_HELLO = 1;
export const MSG_TILES = 2;
export const MSG_STATS = 3;
export const MSG_END = 4;
export const MSG_CLIP = 5;
/** The screen of the computer is off and is being woken (`{on: true}`), or it is awake again (`{on: false}`). */
export const MSG_WAKING = 6;
/** The answer to a MSG_TYPE: `{ok: true}`, or `{ok: false, reason}` with "desktop" (the screen is not a sign-in or a UAC prompt now), "unsupported" or "failed". */
export const MSG_TYPED = 7;
/** Browser to server. */
export const MSG_INPUT = 16;
export const MSG_CLIP_SET = 18;
export const MSG_CLIP_GET = 19;
export const MSG_MONITOR = 20;
export const MSG_PAUSE = 21;
/** Type a text on the sign-in or the UAC screen (a login or a password of a key); never logged. */
export const MSG_TYPE = 22;

/** The biggest text the browser sends to the clipboard of the computer (the payload of a ClipSet message). */
export const CLIP_MAX = 1024 * 1024;

/** WebSocket close code of the panel when the session is already open in another Assistant window. */
export const ASSIST_LIMIT_CODE = 4031;

/** A monitor of the computer; the list has the primary one first. */
export interface Monitor { x: number; y: number; w: number; h: number; primary: boolean }
/** `monitors` may be missing or have one entry; `monitor` is the index of the picked one, -1 for all of them as one picture. */
export interface Hello { w: number; h: number; session: number | string; user: string; monitors?: Monitor[]; monitor?: number; /** "linux" for a Linux computer. */ platform?: string }
/** `desktop` is "default" normally, "winlogon" on the lock, sign-in or UAC screen, "screen-saver" with a screen saver. */
export interface Stats { fps: number; kbps: number; q: number; desktop?: string }
export interface Tile { x: number; y: number; w: number; h: number; jpeg: Uint8Array }

/** The tiles of a Tiles payload: u16 count, then x, y, w, h (u16) and the length (u32, big endian) with the JPEG bytes. Null when the payload is cut or lies about a length. */
export function parseTiles(data: Uint8Array): Tile[] | null {
  if (data.length < 2) return null;
  const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  const count = view.getUint16(0);
  const tiles: Tile[] = [];
  let at = 2;
  for (let i = 0; i < count; i++) {
    if (at + 12 > data.length) return null;
    const len = view.getUint32(at + 8);
    const start = at + 12;
    if (len > data.length - start) return null;
    tiles.push({ x: view.getUint16(at), y: view.getUint16(at + 2), w: view.getUint16(at + 4), h: view.getUint16(at + 6), jpeg: data.subarray(start, start + len) });
    at = start + len;
  }
  return tiles;
}

/** An input event as a message: the type byte, then JSON. */
export function encodeInput(event: Record<string, unknown>): Uint8Array<ArrayBuffer> {
  return withJson(MSG_INPUT, event);
}

/** True when the computer shows something other than the user's desktop (the lock or sign-in screen, a UAC prompt). */
/** The desktop a text may be typed on: the lock screen, the sign-in or a UAC prompt (the helper checks it again when it types). */
export const isSignInDesktop = (desktop: string | undefined | null): boolean => desktop === 'winlogon';

/** The longest text of one typing (a password of a key). */
export const TYPING_MAX = 512;

/** A text to type on the sign-in or UAC screen, then Tab or Enter or nothing. Null when the text is empty, too long or has control characters. */
export function encodeTyping(text: string, after: '' | 'tab' | 'enter', label: string): Uint8Array<ArrayBuffer> | null {
  // eslint-disable-next-line no-control-regex
  if (!text || [...text].length > TYPING_MAX || /[\u0000-\u001f\u007f]/.test(text)) return null;
  return withJson(MSG_TYPE, { text, after: after || undefined, label: [...label.replace(/[\u0000-\u001f\u007f]/g, ' ')].slice(0, 80).join('') });
}

export const isSecureDesktop = (desktop: string | undefined | null): boolean => !!desktop && desktop !== 'default';

/** The monitors of a Hello as a clean list: entries that are not rectangles are dropped. */
export function cleanMonitors(list: unknown): Monitor[] {
  if (!Array.isArray(list)) return [];
  const out: Monitor[] = [];
  for (const m of list) {
    if (!m || typeof m !== 'object') continue;
    const { x, y, w, h, primary } = m as Record<string, unknown>;
    if (!(Number(w) > 0) || !(Number(h) > 0)) continue;
    out.push({ x: Number(x) || 0, y: Number(y) || 0, w: Number(w), h: Number(h), primary: primary === true });
  }
  return out;
}

function withJson(type: number, value: unknown): Uint8Array<ArrayBuffer> {
  const json = new TextEncoder().encode(JSON.stringify(value));
  const out = new Uint8Array(json.length + 1);
  out[0] = type;
  out.set(json, 1);
  return out;
}

/** Picks a monitor: its index, or -1 for all of them as one picture. */
/** Stops the pictures (the tab is hidden) or lets them go on. */
export function encodePause(on: boolean): Uint8Array<ArrayBuffer> {
  return withJson(MSG_PAUSE, { on });
}

export function encodeMonitor(index: number): Uint8Array<ArrayBuffer> {
  return withJson(MSG_MONITOR, { i: Math.trunc(index) });
}

/** The text for the clipboard of the computer. Null when the message would be longer than the limit. */
export function encodeClipSet(text: string): Uint8Array<ArrayBuffer> | null {
  const out = withJson(MSG_CLIP_SET, { text });
  return out.length - 1 > CLIP_MAX ? null : out;
}

/** The request for the clipboard of the computer: the type byte alone. */
export function encodeClipGet(): Uint8Array<ArrayBuffer> {
  return new Uint8Array([MSG_CLIP_GET]);
}

/** Turns wheel deltas into whole notches, keeping the remainder so a smooth trackpad still scrolls. */
export class WheelNotches {
  private fx = 0;
  private fy = 0;

  /** deltaMode follows WheelEvent: 0 pixels (100 a notch), 1 lines (3 a notch), 2 pages (10 notches). Positive is down or right. */
  push(dx: number, dy: number, deltaMode: number): { dx: number; dy: number } {
    const unit = deltaMode === 1 ? 1 / 3 : deltaMode === 2 ? 10 : 1 / 100;
    this.fx += dx * unit;
    this.fy += dy * unit;
    const nx = clampNotch(Math.trunc(this.fx));
    const ny = clampNotch(Math.trunc(this.fy));
    this.fx -= Math.trunc(this.fx);
    this.fy -= Math.trunc(this.fy);
    return { dx: nx, dy: ny };
  }
}

const clampNotch = (n: number): number => Math.max(-10, Math.min(10, n));
