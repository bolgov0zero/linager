<script lang="ts" module>
  export type AssistPhase = 'connecting' | 'open' | 'closed';
  export type AssistScale = 'fit' | 'real' | 'fill';

  /** What the computer reports about the picture; null until the first Hello. */
  export interface AssistInfo {
    w: number;
    h: number;
    user: string;
    /** Frames and traffic of the last second (the Stats message); zero until the first one. */
    fps: number;
    kbps: number;
    q: number;
    /** "default" normally; "winlogon" on the lock, sign-in or UAC screen (see isSecureDesktop). */
    desktop: string;
    /** The monitors of the computer (primary first; empty when it did not say) and the picked one, -1 for all as one picture. */
    monitors: Monitor[];
    monitor: number;
    /** "linux" for a Linux computer: the keys of Windows (Ctrl+Alt+Del, the task manager, Win+R) are not offered. */
    platform: string;
  }
</script>

<script lang="ts">
  import { untrack } from 'svelte';
  import { assistantSocketUrl } from '../api';
  import { t } from '../i18n.svelte';
  import { showToast } from '../toast';
  import {
    ASSIST_LIMIT_CODE,
    MSG_CLIP,
    MSG_TYPED,
    MSG_WAKING,
    MSG_END,
    MSG_HELLO,
    MSG_STATS,
    MSG_TILES,
    WheelNotches,
    cleanMonitors,
    isSignInDesktop,
    encodeClipGet,
    encodeTyping,
    encodeClipSet,
    encodeInput,
    encodeMonitor,
    encodePause,
    parseTiles,
    type Hello,
    type Monitor,
    type Stats,
  } from './protocol';
  import { scanKey, type ScanKey } from './scancodes';
  import { assistSettings, refreshAssistSettings } from './settings';

  /**
   * The screen of a Windows session in the browser: a canvas that gets the picture as JPEG tiles over a WebSocket of
   * the panel (which carries it through the agent) and sends the mouse and the keys back as scan codes. The session
   * is 'console' (what a person sits in front of) or the number of a Windows session; it opens when the component
   * appears and again when the session changes.
   */
  let {
    serverId,
    session,
    phase = $bindable('connecting'),
    info = $bindable(null),
  }: {
    serverId: string;
    session: string;
    phase?: AssistPhase;
    info?: AssistInfo | null;
  } = $props();

  let canvas = $state<HTMLCanvasElement | null>(null);
  let host = $state<HTMLDivElement | null>(null);
  let message = $state('');
  /** The computer's screen is dark and is being woken: a plain "connecting" stub, with no technical hint. */
  let waking = $state(false);
  let mode = $state<AssistScale>('fit');
  let box = $state({ w: 0, h: 0 });
  let size = $state({ w: 0, h: 0 });

  // Handles of the live connection; not reactive.
  let socket: WebSocket | null = null;
  let generation = 0;
  let queue: Promise<void> = Promise.resolve();
  const pressedKeys = new Map<string, ScanKey>();
  const pressedButtons = new Set<number>();
  let last = { x: 0, y: 0 };
  let pendingMove: { x: number; y: number } | null = null;
  let frame = 0;
  const wheel = new WheelNotches();
  /** The tab is hidden (the Console): the pictures are paused. */
  let pausedNow = false;
  /** Set while the clipboard of the computer is asked for: only an answer to that is put into the local clipboard. */
  let clipWanted = 0;
  /** The answer was fetched by a Ctrl+C: an empty clipboard is not worth a message. */
  let clipQuiet = false;

  // The pointer of the computer is part of the picture only when the settings ask for it (off by default); otherwise
  // the local pointer is shown over the screen and moves without lag.
  const showCursor = $derived($assistSettings[serverId]?.show_cursor === true);

  const factor = $derived(mode === 'real' || box.w <= 0 || box.h <= 0 || size.w <= 0 ? 1 : Math.min(box.w / size.w, box.h / size.h));
  // "fill": the picture is stretched to the whole area, its proportions follow the window (the pointer is mapped per axis)
  const fill = $derived(mode === 'fill' && box.w > 0 && box.h > 0);
  const cssW = $derived(fill ? box.w : Math.max(1, Math.floor(size.w * factor)));
  const cssH = $derived(fill ? box.h : Math.max(1, Math.floor(size.h * factor)));

  // ---- the connection ----

  function connect(): void {
    shutdown();
    const mine = ++generation;
    message = '';
    waking = false;
    clipWanted = 0;
    if (!quiet) {
      info = null; // a silent reconnection keeps the last picture and its size until the new screen has come
      size = { w: 0, h: 0 };
    }
    phase = 'connecting';
    const ws = new WebSocket(assistantSocketUrl(serverId, session));
    ws.binaryType = 'arraybuffer';
    socket = ws;
    ws.onopen = () => {
      if (mine === generation && pausedNow) ws.send(encodePause(true)); // the tab was hidden while it connected
    };
    queue = Promise.resolve();
    ws.onmessage = (ev) => {
      if (mine !== generation || !(ev.data instanceof ArrayBuffer)) return;
      const data = new Uint8Array(ev.data);
      // one chain: a frame is drawn completely before the next one is looked at
      queue = queue.then(() => handle(mine, data)).catch(() => undefined);
    };
    ws.onclose = (ev) => {
      if (mine !== generation) return;
      socket = null;
      releaseAll(false);
      if (phase !== 'closed') {
        const neverOpened = phase === 'connecting';
        // The screen was the sign-in and the connection ended: the person has signed in, and the session of the computer is
        // another one now (or is being built). Connect again, patiently: a desktop takes some seconds to come.
        const afterSignIn = !neverOpened && ev.code !== ASSIST_LIMIT_CODE && signedInLately() && autoReconnect(mine, true);
        const again = afterSignIn || (neverOpened && ev.code !== ASSIST_LIMIT_CODE && Date.now() < autoUntil && autoReconnect(mine, autoPatient));
        if (!again) quiet = false;
        end(ev.code === ASSIST_LIMIT_CODE ? t('assist.limit') : ev.code === 1000 ? t('assist.ended') : t('assist.lost'));
      }
    };
  }

  /** Closes the socket without a word; what a message handler still has in the air is dropped. */
  function shutdown(): void {
    generation++;
    cancelAnimationFrame(frame);
    pendingMove = null;
    const ws = socket;
    socket = null;
    if (ws) {
      ws.onmessage = ws.onclose = null;
      try {
        ws.close(1000);
      } catch {
        // already closed
      }
    }
    pressedKeys.clear();
    pressedButtons.clear();
    ctrlFromMeta = false;
    swallowUp.clear();
  }

  // The computer may ask for a new connection (the person signed out, the desktop opened after the sign-in): it is made after a
  // moment, when the old connection is gone and the new session of the computer has appeared. For half a minute after such a
  // request a connection that cannot be opened is tried again; all in all not more than four times in 40 seconds, so that a
  // computer that keeps asking cannot make a loop.
  // While it goes on nothing is said on the screen (no "closed" or "connecting" notice) and the last picture stays: switching
  // between the desktop and the lock screen must look like a change of the picture, not like a failure. The notice appears
  // only when the reconnection gives up.
  // After a sign-in (patient) the new session needs time to appear: up to twelve tries in a minute, two seconds apart.
  // When the screen was the sign-in last, or a key was typed into it: a connection that ends soon after it ends because the
  // person has signed in (the desktop of the computer changes, often to a new session), not because something broke.
  let signInSeen = 0;
  const signedInLately = (): boolean => Date.now() - signInSeen < 30000;
  let autoTimes: number[] = [];
  let autoUntil = 0;
  let autoPatient = false;
  let quiet = $state(false);
  function autoReconnect(mine: number, patient = false): boolean {
    const now = Date.now();
    autoTimes = autoTimes.filter((x) => now - x < (patient ? 60000 : 40000));
    if (autoTimes.length >= (patient ? 12 : 4)) return false;
    autoTimes.push(now);
    autoPatient = patient;
    autoUntil = now + (patient ? 45000 : 30000);
    quiet = true;
    setTimeout(() => {
      if (mine === generation && phase === 'closed') connect();
    }, patient ? 2000 : 1500);
    return true;
  }

  function end(reason: string): void {
    message = reason;
    waking = false;
    phase = 'closed';
    if (!quiet) info = null;
  }

  async function handle(mine: number, data: Uint8Array): Promise<void> {
    if (mine !== generation || data.length === 0) return;
    const body = data.subarray(1);
    switch (data[0]) {
      case MSG_HELLO: {
        const hello = json<Hello>(body);
        if (!hello || !(hello.w > 0) || !(hello.h > 0)) return;
        size = { w: hello.w, h: hello.h };
        if (canvas) {
          canvas.width = hello.w; // resizing clears the picture; the tiles that follow fill it again
          canvas.height = hello.h;
        }
        info = {
          w: hello.w,
          h: hello.h,
          user: hello.user ?? '',
          fps: info?.fps ?? 0,
          kbps: info?.kbps ?? 0,
          q: info?.q ?? 0,
          desktop: info?.desktop ?? 'default',
          monitors: cleanMonitors(hello.monitors),
          monitor: Number.isInteger(hello.monitor) ? (hello.monitor as number) : -1,
          platform: hello.platform ?? '',
        };
        phase = 'open';
        quiet = false;
        waking = false;
        return;
      }
      case MSG_WAKING:
        waking = json<{ on?: boolean }>(body)?.on === true;
        return;
      case MSG_TILES:
        waking = false;
        await draw(mine, body);
        return;
      case MSG_STATS: {
        const s = json<Stats>(body);
        if (s && isSignInDesktop(s.desktop)) signInSeen = Date.now();
        if (s && info) {
          info = { ...info, fps: Number(s.fps) || 0, kbps: Number(s.kbps) || 0, q: Number(s.q) || 0, desktop: typeof s.desktop === 'string' && s.desktop ? s.desktop : 'default' };
        }
        return;
      }
      case MSG_CLIP:
        await takeClip(json<{ text?: string }>(body)?.text);
        return;
      case MSG_TYPED: {
        const r = json<{ ok?: boolean; reason?: string }>(body);
        if (r && r.ok === false) showToast(t(r.reason === 'desktop' ? 'assist.typeDesktop' : r.reason === 'unsupported' ? 'assist.typeUnsupported' : 'assist.typeFailed'));
        return;
      }
      case MSG_END: {
        const e = json<{ reason?: string; reconnect?: boolean }>(body);
        quiet = (e?.reconnect === true && autoReconnect(mine)) || (signedInLately() && autoReconnect(mine, true));
        end(e?.reason || t('assist.ended'));
        return;
      }
    }
  }

  /** The clipboard of the computer came (the answer to getClipboard): it goes into the local clipboard. */
  async function takeClip(text: string | undefined): Promise<void> {
    if (Date.now() > clipWanted) return; // nobody asked: the clipboard of the person is not touched
    clipWanted = 0;
    if (typeof text !== 'string') return;
    if (text === '') {
      if (!clipQuiet) showToast(t('assist.clipEmpty'));
      return;
    }
    try {
      await navigator.clipboard.writeText(text);
      showToast(t('desk.clipboardSaved'));
    } catch {
      showToast(t('assist.clipDenied'));
    }
  }

  function json<T>(bytes: Uint8Array): T | null {
    try {
      return JSON.parse(new TextDecoder().decode(bytes)) as T;
    } catch {
      return null;
    }
  }

  async function draw(mine: number, body: Uint8Array): Promise<void> {
    const ctx = canvas?.getContext('2d');
    const tiles = parseTiles(body);
    if (!ctx || !tiles || size.w === 0) return; // the picture before the first Hello has no size to go to
    const bitmaps = await Promise.all(
      tiles.map((tile) => createImageBitmap(new Blob([tile.jpeg as BlobPart], { type: 'image/jpeg' })).catch(() => null)),
    );
    for (let i = 0; i < bitmaps.length; i++) {
      const bitmap = bitmaps[i];
      if (!bitmap) continue;
      if (mine === generation) ctx.drawImage(bitmap, tiles[i].x, tiles[i].y);
      bitmap.close();
    }
  }

  // The session or the computer changed (or the window appeared): connect again.
  $effect(() => {
    void serverId;
    void session;
    untrack(connect);
    return shutdown;
  });

  // The setting of the pointer is needed as soon as the picture is there (the line under the screen reads it again from time to time).
  $effect(() => {
    if (phase === 'open') void refreshAssistSettings(serverId);
  });

  // ---- input ----

  function send(event: Record<string, unknown>): void {
    if (socket?.readyState === WebSocket.OPEN && phase === 'open') socket.send(encodeInput(event));
  }

  function remote(e: { clientX: number; clientY: number }): { x: number; y: number } {
    const r = canvas?.getBoundingClientRect();
    if (!r || r.width === 0 || r.height === 0) return last;
    const x = Math.max(0, Math.min(size.w - 1, Math.round(((e.clientX - r.left) * size.w) / r.width)));
    const y = Math.max(0, Math.min(size.h - 1, Math.round(((e.clientY - r.top) * size.h) / r.height)));
    last = { x, y };
    return last;
  }

  function flushMove(): void {
    frame = 0;
    if (!pendingMove) return;
    send({ t: 'm', x: pendingMove.x, y: pendingMove.y });
    pendingMove = null;
  }

  function onpointermove(e: PointerEvent): void {
    pendingMove = remote(e);
    if (!frame) frame = requestAnimationFrame(flushMove);
  }

  function onpointerdown(e: PointerEvent): void {
    e.preventDefault();
    canvas?.focus();
    canvas?.setPointerCapture(e.pointerId);
    pressedButtons.add(e.button);
    const p = remote(e);
    send({ t: 'b', b: e.button, d: true, x: p.x, y: p.y });
  }

  function onpointerup(e: PointerEvent): void {
    if (!pressedButtons.delete(e.button)) return;
    const p = remote(e);
    send({ t: 'b', b: e.button, d: false, x: p.x, y: p.y });
  }

  function onwheel(e: WheelEvent): void {
    e.preventDefault();
    const n = wheel.push(e.deltaX, e.deltaY, e.deltaMode);
    if (n.dx !== 0 || n.dy !== 0) send({ t: 'w', dx: n.dx, dy: n.dy });
  }

  const isModifier = (code: string): boolean => /^(Control|Shift|Alt|Meta)(Left|Right)$/.test(code);

  // ---- keyboard: the keys go to the computer as scan codes; the clipboard shortcuts also carry the text ----

  const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
  /** On a Mac Cmd works as Ctrl on the computer (Cmd+C, Cmd+V, Cmd+A...); a Cmd tapped alone is the Windows key. */
  let metaUsed = false;
  let ctrlFromMeta = false;
  let pasteTimer: ReturnType<typeof setTimeout> | null = null;
  let pasteMeta = false;
  /** Key-ups of keys the browser event of which was taken over (the V of a paste is typed by the code below). */
  const swallowUp = new Set<string>();

  const isMeta = (code: string): boolean => code === 'MetaLeft' || code === 'MetaRight';
  const isClipKey = (e: KeyboardEvent): boolean =>
    (e.code === 'KeyC' || e.code === 'KeyV' || e.code === 'KeyX') && !e.altKey && !e.shiftKey && (e.ctrlKey || (isMac && e.metaKey));

  function sendKey(code: string, down: boolean): void {
    const k = scanKey(code);
    if (k) send(down ? { t: 'k', sc: k.sc, ext: k.ext, d: true } : { t: 'k', sc: k.sc, ext: k.ext });
  }

  /** Ctrl on the computer for as long as Cmd is held. */
  function ensureCtrl(): void {
    if (ctrlFromMeta || pressedKeys.has('ControlLeft') || pressedKeys.has('ControlRight')) return;
    ctrlFromMeta = true;
    sendKey('ControlLeft', true);
  }

  function releaseCtrl(): void {
    if (!ctrlFromMeta) return;
    ctrlFromMeta = false;
    sendKey('ControlLeft', false);
  }

  /** Types V for a paste (Ctrl is held on the computer already, or comes from Cmd). */
  function typePaste(): void {
    if (pasteMeta) {
      metaUsed = true;
      ensureCtrl();
    }
    sendKey('KeyV', true);
    sendKey('KeyV', false);
  }

  function onkeydown(e: KeyboardEvent): void {
    if (isMac && isMeta(e.code)) {
      e.preventDefault();
      e.stopPropagation();
      metaUsed = false;
      return;
    }
    const viaMeta = isMac && e.metaKey && !isModifier(e.code);
    if (e.code === 'KeyV' && isClipKey(e)) {
      // Not prevented: the browser then gives the paste event with the text of the local clipboard (no permission asked).
      // The text goes to the computer first, then the V; if the event does not come, the V goes alone.
      e.stopPropagation();
      swallowUp.add(e.code);
      pasteMeta = viaMeta;
      if (pasteTimer) clearTimeout(pasteTimer);
      pasteTimer = setTimeout(() => {
        pasteTimer = null;
        typePaste();
      }, 150);
      return;
    }
    e.preventDefault();
    e.stopPropagation();
    const k = scanKey(e.code);
    if (!k) return;
    if (viaMeta) {
      metaUsed = true;
      ensureCtrl();
    }
    pressedKeys.set(e.code, k);
    send({ t: 'k', sc: k.sc, ext: k.ext, d: true });
    // a copy or a cut: the text of the computer is fetched once the program has put it on the clipboard
    if ((e.code === 'KeyC' || e.code === 'KeyX') && isClipKey(e)) setTimeout(() => getClipboard(true), 300);
  }

  function onpaste(e: ClipboardEvent): void {
    if (document.activeElement !== canvas || !pasteTimer) return;
    e.preventDefault();
    clearTimeout(pasteTimer);
    pasteTimer = null;
    const text = e.clipboardData?.getData('text/plain') ?? '';
    if (text !== '') {
      const msg = encodeClipSet(text);
      if (msg) socket?.send(msg);
      else showToast(t('assist.clipTooBig'));
    }
    typePaste();
  }

  function onkeyup(e: KeyboardEvent): void {
    e.preventDefault();
    e.stopPropagation();
    if (isMac && isMeta(e.code)) {
      // a Mac browser sends no key-up for the keys pressed together with Cmd: let go of them with it
      for (const [code, key] of [...pressedKeys]) {
        if (isModifier(code)) continue;
        pressedKeys.delete(code);
        send({ t: 'k', sc: key.sc, ext: key.ext });
      }
      if (ctrlFromMeta) releaseCtrl();
      else if (!metaUsed) {
        sendKey('MetaLeft', true); // a Cmd tapped alone: the Windows key
        sendKey('MetaLeft', false);
      }
      return;
    }
    if (swallowUp.delete(e.code)) return;
    const k = scanKey(e.code);
    if (!k) return;
    pressedKeys.delete(e.code);
    send({ t: 'k', sc: k.sc, ext: k.ext });
    // a Mac browser sends no key-up for the keys pressed together with Cmd: let go of them with it
    if (e.code === 'MetaLeft' || e.code === 'MetaRight') {
      for (const [code, key] of [...pressedKeys]) {
        if (isModifier(code)) continue;
        pressedKeys.delete(code);
        send({ t: 'k', sc: key.sc, ext: key.ext });
      }
    }
  }

  /** The focus left the screen: whatever is held down on the remote side is let go. */
  function releaseAll(sendIt = true): void {
    if (sendIt) {
      for (const k of pressedKeys.values()) send({ t: 'k', sc: k.sc, ext: k.ext });
      for (const b of pressedButtons) send({ t: 'b', b, d: false, x: last.x, y: last.y });
    }
    pressedKeys.clear();
    pressedButtons.clear();
    if (sendIt) releaseCtrl();
    ctrlFromMeta = false;
    swallowUp.clear();
  }

  // The listeners that must not be passive (the wheel scrolls the page otherwise) and the keys while the screen has the focus.
  $effect(() => {
    const c = canvas;
    if (!c) return;
    const noMenu = (e: Event) => e.preventDefault();
    const blur = () => releaseAll();
    c.addEventListener('wheel', onwheel, { passive: false });
    c.addEventListener('keydown', onkeydown);
    c.addEventListener('keyup', onkeyup);
    c.addEventListener('contextmenu', noMenu);
    c.addEventListener('blur', blur);
    document.addEventListener('paste', onpaste);
    return () => {
      document.removeEventListener('paste', onpaste);
      c.removeEventListener('wheel', onwheel);
      c.removeEventListener('keydown', onkeydown);
      c.removeEventListener('keyup', onkeyup);
      c.removeEventListener('contextmenu', noMenu);
      c.removeEventListener('blur', blur);
    };
  });

  // ---- what the toolbar can do ----

  export function focus(): void {
    canvas?.focus();
  }

  export function disconnect(): void {
    shutdown();
    end(t('assist.disconnected'));
  }

  export function reconnect(): void {
    quiet = false;
    connect();
    focus();
  }

  export function ctrlAltDel(): void {
    send({ t: 'cad' });
    focus();
  }

  /** Picks a monitor of the computer (its index in info.monitors) or -1 for all of them as one picture; the computer answers with a new Hello. */
  /** The tab is hidden: the computer stops sending pictures until it is shown again. */
  export function setPaused(on: boolean): void {
    pausedNow = on;
    if (socket?.readyState === WebSocket.OPEN) socket.send(encodePause(on));
  }

  export function selectMonitor(index: number): void {
    if (socket?.readyState === WebSocket.OPEN && phase === 'open') socket.send(encodeMonitor(index));
    focus();
  }

  export const clipboardAvailable = (): boolean => typeof navigator !== 'undefined' && !!navigator.clipboard?.readText && !!navigator.clipboard?.writeText;

  /** The local clipboard (text) to the computer; the browser asks for permission. */
  export async function sendClipboard(): Promise<void> {
    if (socket?.readyState !== WebSocket.OPEN || phase !== 'open') return;
    try {
      const msg = encodeClipSet(await navigator.clipboard.readText());
      if (!msg) {
        showToast(t('assist.clipTooBig'));
        return;
      }
      socket?.send(msg);
      showToast(t('assist.clipSent'));
    } catch {
      showToast(t('assist.clipDenied'));
    }
  }

  /** Asks for the clipboard (text) of the computer; it goes into the local one when the answer comes. */
  export function getClipboard(quiet = false): void {
    if (socket?.readyState !== WebSocket.OPEN || phase !== 'open') return;
    clipWanted = Date.now() + 15000;
    clipQuiet = quiet;
    socket.send(encodeClipGet());
  }

  /** Presses the keys one after the other and lets them go in reverse; the codes are KeyboardEvent.code names. */
  export function combo(codes: string[]): void {
    const keys = codes.map((c) => scanKey(c)).filter((k): k is ScanKey => k !== null);
    for (const k of keys) send({ t: 'k', sc: k.sc, ext: k.ext, d: true });
    for (const k of [...keys].reverse()) send({ t: 'k', sc: k.sc, ext: k.ext });
    focus();
  }

  /** Types a text on the sign-in or UAC screen (a login or a password of a key), then Tab, Enter or nothing. False when it was not sent. */
  export function typeText(text: string, after: '' | 'tab' | 'enter', label: string): boolean {
    if (socket?.readyState !== WebSocket.OPEN || phase !== 'open') return false;
    const msg = encodeTyping(text, after, label);
    if (!msg) return false;
    socket.send(msg);
    signInSeen = Date.now();
    return true;
  }

  export function winKey(): void {
    combo(['MetaLeft']);
  }

  export function setScale(kind: AssistScale): void {
    mode = kind;
  }

  const label = $derived(session === 'console' ? t('assist.console') : t('assist.sessionN', { n: session }));
</script>

<div class="asst" bind:this={host}>
  <div class="asst-view" class:real={mode === 'real'} bind:clientWidth={box.w} bind:clientHeight={box.h}>
    <canvas
      bind:this={canvas}
      tabindex="0"
      aria-label={t('assist.screenLabel', { name: label })}
      style:width="{cssW}px"
      style:height="{cssH}px"
      style:cursor={showCursor ? 'none' : 'default'}
      {onpointermove}
      {onpointerdown}
      {onpointerup}
      onpointercancel={onpointerup}
    ></canvas>
  </div>

  {#if (phase !== 'open' && !quiet) || (waking && phase !== 'closed')}
    <div class="veil" role={phase === 'closed' ? 'alert' : 'status'}>
      {#if phase === 'connecting' || waking}
        <div class="spin" aria-hidden="true"><i></i><i></i><i></i><i></i></div>
        <p>{t('assist.connecting', { name: label })}</p>
        <p class="small hint">{t(waking ? 'assist.wakingHint' : 'assist.connectingHint')}</p>
      {:else}
        <h3>{t('assist.closedTitle')}</h3>
        <p class="reason">{message}</p>
        <button class="btn primary" type="button" onclick={reconnect}>{t('assist.reconnect')}</button>
      {/if}
    </div>
  {/if}
</div>
