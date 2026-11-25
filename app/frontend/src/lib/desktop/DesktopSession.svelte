<script lang="ts" module>
  export type DeskPhase = 'form' | 'connecting' | 'open' | 'closed';

  /** WebSocket close code of the server when a remote desktop of the computer is already open. */
  export const DESKTOP_LIMIT_CODE = 4030;

  /** The rdp client wants these numbers for the scale: fit the window, fill it, or show the real pixels. */
  export const SCALE = { fit: 1, full: 2, real: 3 } as const;
</script>

<script lang="ts">
  import { onDestroy } from 'svelte';
  import { attachEdgeScroll } from '../edge-scroll';
  import KeyPicker from '../keys/KeyPicker.svelte';
  import { getPref, setPref } from '../prefs';
  import { desktopApi, desktopSocketUrl, type DesktopState } from '../api';
  import { t } from '../i18n.svelte';
  import { showToast } from '../toast';
  import { planKey } from './keys';

  /**
   * The remote desktop of a Windows computer: a login form, then the screen of the browser RDP client (IronRDP,
   * WebAssembly, loaded when it is first needed). The client talks RDCleanPath over a WebSocket to the panel, which
   * carries it through the agent to the Remote Desktop service of the computer. The password goes to the client
   * only; it is not stored and not sent to the panel in the clear.
   */
  let {
    serverId,
    address,
    phase = $bindable('form'),
    autoResize = $bindable(true),
    onended,
  }: {
    serverId: string;
    /** How the computer is called inside the connection (its address in the panel). */
    address: string;
    phase?: DeskPhase;
    /** Ask the computer to change its resolution when the window changes. */
    autoResize?: boolean;
    onended?: () => void;
  } = $props();

  const loginKey = () => `linager.desktop.login.${serverId}`;
  function savedLogin(): string {
    return getPref(loginKey()) ?? '';
  }

  let login = $state(savedLogin());
  let password = $state('');
  let domain = $state('');
  /** The name of the key of the vault the form was filled from (the password is not shown, only where it came from). */
  let fromKey = $state<string | null>(null);
  let message = $state('');
  let hostState = $state<DesktopState | null>(null);
  let enabling = $state(false);
  let host = $state<HTMLDivElement | null>(null);

  // Handles of the live client; not reactive.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let ui: any = null;
  let element: HTMLElement | null = null;
  let resizeTimer: ReturnType<typeof setTimeout> | undefined;
  let observer: ResizeObserver | undefined;
  let gone = false;

  // ---- the keyboard: Cmd of a Mac works as Ctrl, and the clipboard shortcuts carry the clipboard with them ----

  const mac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
  /** Cmd was used with another key: letting it go is not a tap of the Windows key. */
  let metaUsed = false;
  let clipWarned = false;
  let keysOn = false;

  const wait = (ms: number): Promise<void> => new Promise((resolve) => setTimeout(resolve, ms));

  /** A key event for the client (it listens on the window): with Ctrl (typed as well when it comes from Cmd) and the shift of the real key. */
  function typeKey(e: KeyboardEvent, withCtrl: boolean, full: boolean): void {
    const fire = (type: 'keydown' | 'keyup', code: string, key: string): void => {
      window.dispatchEvent(new KeyboardEvent(type, { code, key, ctrlKey: withCtrl || e.ctrlKey, shiftKey: e.shiftKey, bubbles: true, cancelable: true }));
    };
    if (full) fire('keydown', 'ControlLeft', 'Control');
    fire('keydown', e.code, e.key);
    fire('keyup', e.code, e.key);
    if (full) fire('keyup', 'ControlLeft', 'Control');
  }

  /** The local clipboard goes to the computer first (the keystroke is a user action, so the browser may ask), then the paste. */
  async function paste(e: KeyboardEvent, viaMeta: boolean): Promise<void> {
    try {
      await ui?.sendClipboardData();
    } catch (err) {
      const text = String(err instanceof Error ? err.message : err);
      // an empty clipboard is no news; a refusal is, once
      if (!clipWarned && !/no data/i.test(text)) {
        clipWarned = true;
        showToast(t('desk.clipDenied'));
      }
    }
    await wait(120);
    if (ui) typeKey(e, viaMeta, viaMeta);
  }

  /** The clipboard of the computer goes into the local one once the program has put the text there. */
  function fetchClipboard(): void {
    setTimeout(() => {
      void Promise.resolve()
        .then(() => ui?.saveRemoteClipboardData())
        .catch(() => undefined); // nothing came (the browser is still asking, or there is nothing new): nothing to say
    }, 350);
  }

  function onKey(e: KeyboardEvent): void {
    if (!e.isTrusted || !ui || !element || document.activeElement !== element) return;
    const plan = planKey(e, mac);
    const stop = (): void => {
      e.preventDefault();
      e.stopImmediatePropagation();
    };
    if (plan === 'meta') {
      stop();
      if (e.type === 'keydown') {
        if (!e.repeat) metaUsed = false;
      } else if (!metaUsed) {
        try {
          ui.metaKey();
        } catch {
          // not connected any more
        }
      }
      return;
    }
    if (plan === 'pass' || e.type !== 'keydown') return;
    const viaMeta = mac && e.metaKey;
    if (plan === 'copy' && !viaMeta) {
      fetchClipboard(); // Ctrl+C itself goes as it is
      return;
    }
    stop();
    if (viaMeta) metaUsed = true;
    if (plan === 'paste') {
      void paste(e, viaMeta);
      return;
    }
    typeKey(e, true, true);
    if (plan === 'copy') fetchClipboard();
  }

  function keys(on: boolean): void {
    if (on === keysOn) return;
    keysOn = on;
    for (const type of ['keydown', 'keyup'] as const) {
      if (on) window.addEventListener(type, onKey, true);
      else window.removeEventListener(type, onKey, true);
    }
  }

  $effect(() => {
    if (phase !== 'form') return;
    void desktopApi.state(serverId).then((s) => (hostState = s)).catch(() => (hostState = null));
  });

  async function enable(): Promise<void> {
    enabling = true;
    try {
      hostState = await desktopApi.enable(serverId);
      showToast(t('desk.enabled'));
    } catch (e) {
      message = e instanceof Error ? e.message : t('common.genericError');
    } finally {
      enabling = false;
    }
  }

  /** The words for what the client reports when the connection fails. */
  function explain(error: unknown): string {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const e = error as any;
    let kind = -1;
    try {
      kind = typeof e?.kind === 'function' ? e.kind() : -1;
    } catch {
      kind = -1;
    }
    switch (kind) {
      case 1:
      case 2:
        return t('desk.err.logon');
      case 3:
        return t('desk.err.denied');
      case 4:
      case 5:
        return t('desk.err.proxy');
      case 6:
        return t('desk.err.negotiation');
    }
    const text = typeof e?.backtrace === 'function' ? String(e.backtrace()) : e instanceof Error ? e.message : String(e ?? '');
    return text.split('\n')[0] || t('common.genericError');
  }

  function size(): { width: number; height: number } {
    const w = host?.clientWidth || 1024;
    const h = host?.clientHeight || 700;
    const even = (n: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, n - (n % 2)));
    return { width: even(w, 640, 4096), height: even(h, 480, 4096) };
  }

  /** Fills the form from a key of the vault (the Keys part) and signs in at once. */
  export function useKey(entry: { login: string; domain: string; password: string; name: string }): void {
    login = entry.login;
    domain = entry.domain;
    password = entry.password;
    fromKey = entry.name;
    void connect();
  }

  export async function connect(): Promise<void> {
    if (!login.trim() || !password) return;
    message = '';
    phase = 'connecting';
    try {
      const [rdp] = await Promise.all([import('@devolutions/iron-remote-desktop-rdp'), import('@devolutions/iron-remote-desktop')]);
      if (gone || !host) return;
      await rdp.init('warn'); // loads the WebAssembly module
      if (gone || !host) return;
      element = document.createElement('iron-remote-desktop');
      element.setAttribute('scale', 'fit');
      element.setAttribute('verbose', 'false');
      element.setAttribute('flexcenter', 'true');
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (element as any).module = rdp.Backend;
      const ready = new Promise<void>((resolve) => {
        element?.addEventListener('ready', (ev) => {
          ui = (ev as CustomEvent).detail?.irgUserInteraction ?? null;
          resolve();
        });
      });
      host.appendChild(element);
      await ready;
      ui.setEnableClipboard(true);
      ui.setEnableAutoClipboard(true);
      clipWarned = false;
      keys(true);
      ui.onWarningCallback((w: string) => showToast(w));
      const config = ui
        .configBuilder()
        .withUsername(login.trim())
        .withPassword(password)
        .withServerDomain(domain.trim())
        .withDestination(`${address}:${hostState?.port ?? 3389}`)
        .withProxyAddress(desktopSocketUrl(serverId, login.trim()))
        .withAuthToken('linager') // the panel signs the person in by the session cookie of the same page
        .withDesktopSize(size())
        .withExtension(rdp.displayControl(true))
        .build();
      const info = await ui.connect(config);
      ui.setVisibility(true); // the screen of the client stays hidden until it is asked to show
      setPref(loginKey(), login.trim());
      password = '';
      fromKey = null;
      phase = 'open';
      watchSize();
      void info.run().then(
        (end: { reason: () => string }) => finish(end.reason()),
        (err: unknown) => finish(explain(err)),
      );
    } catch (err) {
      cleanup();
      message = explain(err);
      phase = 'form';
    }
  }

  function finish(reason: string): void {
    if (gone) return;
    cleanup();
    message = reason;
    phase = 'closed';
    onended?.();
  }

  function cleanup(): void {
    keys(false);
    observer?.disconnect();
    clearTimeout(resizeTimer);
    try {
      ui?.shutdown();
    } catch {
      // already closed
    }
    element?.remove();
    element = null;
    ui = null;
  }

  /** The window changed size: the computer changes its resolution with it (when that is switched on). */
  function watchSize(): void {
    if (!host) return;
    observer = new ResizeObserver(() => {
      if (!autoResize) return;
      clearTimeout(resizeTimer);
      resizeTimer = setTimeout(() => {
        const s = size();
        try {
          ui?.resize(s.width, s.height);
        } catch {
          // the computer does not support it
        }
      }, 350);
    });
    observer.observe(host);
  }

  // ---- what the toolbar can do ----

  export function disconnect(): void {
    cleanup();
    message = '';
    phase = 'form';
  }

  export function focus(): void {
    element?.focus();
  }

  export function ctrlAltDel(): void {
    ui?.ctrlAltDel();
    focus();
  }

  export function winKey(): void {
    ui?.metaKey();
    focus();
  }

  /** Presses and releases keys together, named by KeyboardEvent.code. The client reads them from the page's key events. */
  export function combo(codes: string[]): void {
    if (!ui) return;
    focus();
    const send = (type: 'keydown' | 'keyup', code: string, held: Set<string>) =>
      window.dispatchEvent(
        new KeyboardEvent(type, {
          code,
          key: code.replace(/^(Key|Digit)/, '').replace(/(Left|Right)$/, ''),
          ctrlKey: held.has('ControlLeft'),
          altKey: held.has('AltLeft'),
          shiftKey: held.has('ShiftLeft'),
          metaKey: held.has('MetaLeft'),
          bubbles: true,
          cancelable: true,
        }),
      );
    const held = new Set<string>();
    for (const c of codes) {
      held.add(c);
      send('keydown', c, held);
    }
    for (const c of [...codes].reverse()) {
      send('keyup', c, held);
      held.delete(c);
    }
  }

  let scaleKind: keyof typeof SCALE = 'fit';
  let hideBars: HTMLStyleElement | null = null;
  export function setScale(kind: keyof typeof SCALE): void {
    scaleKind = kind;
    ui?.setScale(SCALE[kind]);
    // in "100%" the picture is scrolled by the mouse at the edges, so the scroll bars of the client (in its shadow root) are hidden
    const root = element?.shadowRoot;
    if (!root) return;
    if (kind === 'real' && !hideBars) {
      hideBars = document.createElement('style');
      hideBars.textContent = '*{scrollbar-width:none}*::-webkit-scrollbar{display:none}';
      root.appendChild(hideBars);
    } else if (kind !== 'real' && hideBars) {
      hideBars.remove();
      hideBars = null;
    }
  }

  /** The part of the client that scrolls in "100%": the client keeps it in its shadow root. */
  function scroller(): HTMLElement | null {
    const root = element?.shadowRoot;
    if (!root) return null;
    for (const el of root.querySelectorAll<HTMLElement>('div')) {
      if (el.scrollWidth <= el.clientWidth + 1 && el.scrollHeight <= el.clientHeight + 1) continue;
      const o = getComputedStyle(el).overflow;
      if (o !== 'visible' && o !== 'hidden') return el;
    }
    return null;
  }
  $effect(() => {
    if (!host) return;
    return attachEdgeScroll(host, { scroller, enabled: () => scaleKind === 'real' });
  });

  /** The local clipboard to the computer (the browser asks for permission). */
  export async function sendClipboard(): Promise<void> {
    try {
      await ui?.sendClipboardData();
    } catch (e) {
      showToast(explain(e));
    }
  }

  /** The clipboard of the computer to the local one. */
  export async function saveClipboard(): Promise<void> {
    try {
      await ui?.saveRemoteClipboardData();
      showToast(t('desk.clipboardSaved'));
    } catch (e) {
      showToast(explain(e));
    }
  }

  export const clipboardAvailable = (): boolean => typeof navigator !== 'undefined' && !!navigator.clipboard?.readText;

  onDestroy(() => {
    gone = true;
    cleanup();
  });

  const needsEnable = $derived(hostState !== null && (!hostState.enabled || !hostState.listening));
</script>

<div class="desk">
  <div class="screen" bind:this={host} class:live={phase === 'open' || phase === 'connecting'}></div>
  {#if phase !== 'open'}
    <form class="desk-form" onsubmit={(e) => { e.preventDefault(); void connect(); }}>
      <h3>{t('desk.title')}</h3>
      {#if phase === 'connecting'}
        <p class="dim">{t('desk.connecting')}</p>
      {:else}
        {#if hostState && !hostState.supported}
          <div class="banner">{t('desk.unsupported', { edition: hostState.edition })}</div>
        {:else if needsEnable}
          <div class="banner">{hostState && !hostState.enabled ? t('desk.off') : t('desk.notListening')}</div>
          <button class="btn" type="button" disabled={enabling} onclick={() => void enable()}>{t('desk.enable')}</button>
        {/if}
        <KeyPicker {serverId} use="rdp" picked={fromKey} onpick={useKey} />
        <label class="fl"><span>{t('desk.login')}</span><span class="field"><input bind:value={login} autocomplete="username" spellcheck="false" autocapitalize="off" /></span></label>
        <label class="fl"><span>{t('desk.password')}</span><span class="field"><input type="password" bind:value={password} autocomplete="current-password" /></span></label>
        <label class="fl"><span>{t('desk.domain')} <i class="dim">{t('desk.optional')}</i></span><span class="field"><input bind:value={domain} spellcheck="false" autocapitalize="off" /></span></label>
        {#if message}<div class="err" role="alert">{message}</div>{/if}
        <button class="btn primary" type="submit" disabled={!login.trim() || !password}>{phase === 'closed' ? t('desk.reconnect') : t('desk.connect')}</button>
        <p class="dim">{t('desk.note')}</p>
      {/if}
    </form>
  {/if}
</div>
