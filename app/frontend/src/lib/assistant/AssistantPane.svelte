<script lang="ts">
  import { assistantApi, type AssistSession } from '../api';
  import ContextMenu, { type CtxItem } from '../ContextMenu.svelte';
  import { t } from '../i18n.svelte';
  import FloatingBar from '../FloatingBar.svelte';
  import { createFullscreen } from '../fullscreen.svelte';
  import Icon from '../Icon.svelte';
  import { native } from '../native';
  import AssistantBadge from './AssistantBadge.svelte';
  import AssistantQuality from './AssistantQuality.svelte';
  import AssistantSessionView, { type AssistInfo, type AssistPhase } from './AssistantSession.svelte';
  import KeyPicker from '../keys/KeyPicker.svelte';
  import { accountOf, type Entry } from '../keys/vault';
  import { showToast } from '../toast';
  import AssistantStats from './AssistantStats.svelte';
  import { isSignInDesktop } from './protocol';
  import { assistInfoShown, toggleAssistInfo } from './settings';

  // The Assistant of a computer: the bar (the session, the display, Ctrl+Alt+Del, the menu), the screen and the line under it.
  // Used in a Console tab and on the server page. A hidden pane stops the pictures (paused).
  let { serverId, name, online, paused = false, nativeWindows = false, onwindow }: { serverId: string; name: string; online: boolean; paused?: boolean; nativeWindows?: boolean; onwindow?: () => void } = $props();

  let phase = $state<AssistPhase>('connecting');
  let info = $state<AssistInfo | null>(null);
  let screen = $state<AssistantSessionView | null>(null);
  let session = $state('console');
  let display = $state<'fit' | 'fill' | 'real'>('fit');
  let list = $state<AssistSession[]>([]);
  let menu = $state<{ x: number; y: number; items: CtxItem[] } | null>(null);
  let quality = $state(false);
  let root = $state<HTMLDivElement | null>(null);
  const full = createFullscreen({ native: () => nativeWindows, root: () => root });

  $effect(() => screen?.setPaused(paused));
  $effect(() => screen?.setScale(display));

  async function load(): Promise<void> {
    try {
      list = (await assistantApi.sessions(serverId)).sessions;
    } catch {
      // the list stays as it was
    }
  }
  $effect(() => {
    void serverId;
    void load();
    const i = setInterval(() => void load(), 15000);
    return () => clearInterval(i);
  });

  const live = $derived(phase === 'open');
  const linux = $derived(info?.platform === 'linux');
  const entries = $derived.by(() => {
    const out: { value: string; s?: AssistSession }[] = list.map((s) => ({ value: s.console ? 'console' : String(s.id), s }));
    if (!out.some((e) => e.value === 'console')) out.unshift({ value: 'console' });
    return out;
  });
  const current = $derived(entries.find((e) => e.value === session)?.s);
  const canCad = $derived((current?.type ?? (session === 'console' ? 'console' : '')) === 'console');
  const statusText = $derived(phase === 'connecting' ? t('assist.connectingShort') : phase === 'open' ? t('assist.open') : t('assist.closed'));
  /** The name of a session without the computer or the domain ("HOMESRV\\bolgov" is "bolgov"). */
  const userOf = (u: string): string => u.slice(u.lastIndexOf('\\') + 1);
  function open(items: CtxItem[], ev: MouseEvent): void {
    const r = (ev.currentTarget as HTMLElement).getBoundingClientRect();
    menu = { x: r.left, y: r.bottom + 2, items };
  }
  // a row of the menu "Session": the number, the user (the address of the RDP client under it) and the kind of the session as a badge
  const sessionItems = $derived<CtxItem[]>(entries.map((e) => {
    const s = e.s;
    if (!s) return { key: e.value, icon: 'log-in', label: t('assist.console'), selected: e.value === session, onselect: () => (session = e.value) };
    const user = userOf(s.user);
    return {
      key: e.value, icon: user ? 'user' : 'log-in', lead: String(s.id), label: user || t('assist.noUser'), sub: s.type === 'rdp' && s.client ? s.client : undefined,
      badge: s.screen ? { text: t(`assist.type.${s.type}`), tone: s.type === 'rdp' ? 'info' : 'mute' } : { text: t('assist.noScreen'), tone: 'warn' },
      selected: e.value === session, disabled: !s.screen, onselect: () => (session = e.value),
    } as CtxItem;
  }));
  /** The window grows or shrinks to the size of the picture (the window of the Assistant only: the pane inside the main window does not resize it). */
  async function fitWindow(): Promise<void> {
    const d = screen?.fitDelta();
    if (!d) return;
    try {
      if (nativeWindows) await native.fitWindow(d.dw, d.dh);
      else window.resizeTo(window.outerWidth + d.dw, window.outerHeight + d.dh);
    } catch {
      showToast(t('common.genericError'));
    }
  }
  const displayItems = $derived<CtxItem[]>([
    { key: 'fit', icon: 'scaling', label: t('assist.mode.fit'), selected: display === 'fit', onselect: () => (display = 'fit') },
    { key: 'fill', icon: 'expand', label: t('assist.mode.fill'), selected: display === 'fill', onselect: () => (display = 'fill') },
    { key: 'real', icon: 'scan', label: t('assist.mode.real'), selected: display === 'real', onselect: () => (display = 'real') },
    ...(onwindow || !live ? [] : [{ key: 'fitwin', icon: 'proportions', label: t('assist.mode.window'), separated: true, disabled: full.on, onselect: () => void fitWindow() }]),
    { key: 'full', icon: full.on ? 'minimize-2' : 'maximize', label: full.on ? t('fs.exit') : t('assist.mode.full'), separated: true, onselect: () => void full.toggle() },
  ]);
  const moreItems = $derived.by<CtxItem[]>(() => {
    const s = screen;
    const clip = s?.clipboardAvailable() ?? false;
    const mons = info?.monitors ?? [];
    const picked = info?.monitor ?? -1;
    const monitors: CtxItem[] = mons.length < 2 ? [] : [
      ...mons.map((m, i) => ({ key: `mon${i}`, icon: 'monitor', label: `${t('assist.monitorN', { n: i + 1 })}${m.primary ? ` (${t('assist.monitorPrimary')})` : ''} · ${m.w}×${m.h}`, selected: picked === i, separated: i === 0, onselect: () => s?.selectMonitor(i) })),
      { key: 'monall', icon: 'monitor', label: t('assist.monitorAll'), selected: picked === -1, onselect: () => s?.selectMonitor(-1) },
    ];
    return [
      ...(linux ? [] : [{ key: 'tm', icon: 'activity', label: t('assist.taskManager'), hint: 'Ctrl+Shift+Esc', onselect: () => s?.combo(['ControlLeft', 'ShiftLeft', 'Escape']) }]),
      { key: 'win', icon: 'layout-grid', label: t('assist.winKey'), hint: 'Win', onselect: () => s?.winKey() },
      { key: 'lock', icon: 'lock', label: t('assist.lock'), hint: 'Win+L', onselect: () => s?.combo(['MetaLeft', 'KeyL']) },
      ...(linux ? [] : [{ key: 'run', icon: 'play', label: t('assist.run'), hint: 'Win+R', onselect: () => s?.combo(['MetaLeft', 'KeyR']) }]),
      { key: 'alttab', icon: 'arrow-left-right', label: t('assist.altTab'), hint: 'Alt+Tab', onselect: () => s?.combo(['AltLeft', 'Tab']) },
      ...monitors,
      { key: 'send', icon: 'clipboard-paste', label: t('desk.clipSend'), separated: true, disabled: !clip, onselect: () => void s?.sendClipboard() },
      { key: 'save', icon: 'clipboard-copy', label: t('desk.clipSave'), disabled: !clip, onselect: () => s?.getClipboard() },
      { key: 'info', icon: 'info', label: t('assist.info'), selected: $assistInfoShown, separated: true, onselect: toggleAssistInfo },
      { key: 'quality', icon: 'sliders-horizontal', label: t('assist.quality'), onselect: () => (quality = true) },
      { key: 'off', icon: 'unplug', label: t('assist.disconnect'), danger: true, separated: true, onselect: () => s?.disconnect() },
    ];
  });
  const canType = $derived(live && !linux && isSignInDesktop(info?.desktop));
  /** Types the login, the password, or both (login, Tab, password, Enter) of a key on the sign-in or UAC screen. */
  function typeKey(e: Entry, action: 'login' | 'password' | 'signin'): void {
    const s = screen;
    if (!s) return;
    const sent = action === 'login' ? s.typeText(accountOf(e), 'tab', e.name) : action === 'password' ? s.typeText(e.password, 'enter', e.name) : s.typeText(accountOf(e), 'tab', e.name) && s.typeText(e.password, 'enter', e.name);
    if (!sent) showToast(t('assist.typeFailed'));
    s.focus();
  }
</script>

{#snippet bar()}
    <span class="dot" class:ok={online && phase === 'open'}></span>
    <b>{t('assist.header', { name })}</b>{#if online}<span class="dim">· {statusText}</span>{/if}
    {#if online && phase === 'open'}<AssistantBadge desktop={info?.desktop} />{/if}
    <span class="grow"></span>
    {#if online}
      <button class="btn sm" type="button" onclick={(e) => open(sessionItems, e)}>{t('assist.sessionBtn')}<Icon name="chevron-down" size={13} /></button>
      <button class="btn sm icon" type="button" title={t('assist.display')} aria-label={t('assist.display')} onclick={(e) => open(displayItems, e)}><Icon name="monitor" size={14} /></button>
      {#if live}
        {#if canType}<KeyPicker {serverId} use="uac" variant="icon" title={t('keys.act.title')} onact={typeKey} />{/if}
        {#if !linux}<button class="btn sm icon" type="button" title={canCad ? t('assist.cadHint') : t('assist.cadConsoleOnly')} aria-label="Ctrl+Alt+Del" disabled={!canCad} onclick={() => screen?.ctrlAltDel()}><Icon name="keyboard" size={14} /></button>{/if}
        <button class="btn sm" type="button" onclick={(e) => open(moreItems, e)}>{t('assist.more')}<Icon name="chevron-down" size={13} /></button>
      {/if}
    {/if}
    <button class="btn sm icon" type="button" title={t('term.files')} aria-label={t('term.files')} onclick={() => native.openWindow('files', serverId, name, nativeWindows)}><Icon name="folder" size={14} /></button>
    <button class="btn sm icon" type="button" title={t('term.manager')} aria-label={t('term.manager')} onclick={() => native.openWindow('manager', serverId, name, nativeWindows)}><Icon name="activity" size={14} /></button>
    {#if full.on}<button class="btn sm icon" type="button" title={t('fs.exit')} aria-label={t('fs.exit')} onclick={() => void full.set(false)}><Icon name="minimize-2" size={14} /></button>
    {:else if onwindow}<button class="btn sm icon" type="button" title={t('pult.inWindow')} aria-label={t('pult.inWindow')} onclick={onwindow}><Icon name="external-link" size={14} /></button>{/if}
{/snippet}

<div class="asst-pane" class:fs={full.on} bind:this={root}>
  {#if full.on}<FloatingBar>{@render bar()}</FloatingBar>{:else}<div class="asst-bar">{@render bar()}</div>{/if}
  {#if !online}<p class="dim pad">{t('assist.offline')}</p>
  {:else}
    <div class="asst-box">
      <div class="asst-screen"><AssistantSessionView {serverId} {session} bind:phase bind:info bind:this={screen} /></div>
      {#if $assistInfoShown && !full.on}<AssistantStats {serverId} {phase} {info} onsettings={() => (quality = true)} />{/if}
    </div>
  {/if}
</div>
{#if menu}<ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />{/if}
{#if quality}<AssistantQuality {serverId} onclose={() => (quality = false)} />{/if}
