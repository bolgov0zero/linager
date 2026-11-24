<script lang="ts">
  import { assistantApi, type AssistSession } from '../api';
  import ContextMenu, { type CtxItem } from '../ContextMenu.svelte';
  import { t } from '../i18n.svelte';
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
  const label = (s: AssistSession): string => {
    const note = !s.screen ? t('assist.noScreen') : s.type === 'rdp' && s.client ? s.client : '';
    return `${s.id} · ${t(`assist.type.${s.type}`)} · ${s.user || t('assist.noUser')}${note ? ` (${note})` : ''}`;
  };

  function open(items: CtxItem[], ev: MouseEvent): void {
    const r = (ev.currentTarget as HTMLElement).getBoundingClientRect();
    menu = { x: r.left, y: r.bottom + 2, items };
  }
  const sessionItems = $derived<CtxItem[]>(entries.map((e) => ({ key: e.value, label: `${e.value === session ? '✓ ' : ''}${e.s ? label(e.s) : t('assist.console')}`, disabled: e.s ? !e.s.screen : false, onselect: () => (session = e.value) })));
  const displayItems = $derived<CtxItem[]>([
    { key: 'fit', label: `${display === 'fit' ? '✓ ' : ''}${t('assist.mode.fit')}`, onselect: () => (display = 'fit') },
    { key: 'fill', label: `${display === 'fill' ? '✓ ' : ''}${t('assist.mode.fill')}`, onselect: () => (display = 'fill') },
    { key: 'real', label: `${display === 'real' ? '✓ ' : ''}${t('assist.mode.real')}`, onselect: () => (display = 'real') },
    { key: 'full', label: t('assist.mode.full'), separated: true, onselect: () => screen?.fullscreen() },
  ]);
  const moreItems = $derived.by<CtxItem[]>(() => {
    const s = screen;
    const clip = s?.clipboardAvailable() ?? false;
    const mons = info?.monitors ?? [];
    const picked = info?.monitor ?? -1;
    const monitors: CtxItem[] = mons.length < 2 ? [] : [
      ...mons.map((m, i) => ({ key: `mon${i}`, label: `${picked === i ? '✓ ' : ''}${t('assist.monitorN', { n: i + 1 })}${m.primary ? ` (${t('assist.monitorPrimary')})` : ''} · ${m.w}×${m.h}`, separated: i === 0, onselect: () => s?.selectMonitor(i) })),
      { key: 'monall', label: `${picked === -1 ? '✓ ' : ''}${t('assist.monitorAll')}`, onselect: () => s?.selectMonitor(-1) },
    ];
    return [
      ...(linux ? [] : [{ key: 'tm', label: t('assist.taskManager'), onselect: () => s?.combo(['ControlLeft', 'ShiftLeft', 'Escape']) }]),
      { key: 'win', label: t('assist.winKey'), onselect: () => s?.winKey() },
      { key: 'lock', label: t('assist.lock'), onselect: () => s?.combo(['MetaLeft', 'KeyL']) },
      ...(linux ? [] : [{ key: 'run', label: t('assist.run'), onselect: () => s?.combo(['MetaLeft', 'KeyR']) }]),
      { key: 'alttab', label: t('assist.altTab'), onselect: () => s?.combo(['AltLeft', 'Tab']) },
      ...monitors,
      { key: 'send', label: t('desk.clipSend'), separated: true, disabled: !clip, onselect: () => void s?.sendClipboard() },
      { key: 'save', label: t('desk.clipSave'), disabled: !clip, onselect: () => s?.getClipboard() },
      { key: 'info', label: `${$assistInfoShown ? '✓ ' : ''}${t('assist.info')}`, separated: true, onselect: toggleAssistInfo },
      { key: 'quality', label: t('assist.quality'), onselect: () => (quality = true) },
      { key: 'off', label: t('assist.disconnect'), onselect: () => s?.disconnect() },
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

<div class="asst-pane">
  <div class="asst-bar">
    <span class="dot" class:ok={online && phase === 'open'}></span>
    <b>{t('assist.header', { name })}</b>{#if online}<span class="dim">· {statusText}</span>{/if}
    {#if online && phase === 'open'}<AssistantBadge desktop={info?.desktop} />{/if}
    <span class="grow"></span>
    {#if online}
      <button class="btn sm" type="button" onclick={(e) => open(sessionItems, e)}>{t('assist.sessionBtn')}<Icon name="chevron-down" size={13} /></button>
      <button class="btn sm icon" type="button" title={t('assist.display')} aria-label={t('assist.display')} onclick={(e) => open(displayItems, e)}><Icon name="monitor" size={14} /></button>
      {#if live}
        {#if canType}<KeyPicker {serverId} use="uac" variant="icon" title={t('keys.act.title')} onact={typeKey} />{/if}
        {#if !linux}<button class="btn sm" type="button" title={canCad ? t('assist.cadHint') : t('assist.cadConsoleOnly')} disabled={!canCad} onclick={() => screen?.ctrlAltDel()}>Ctrl+Alt+Del</button>{/if}
        <button class="btn sm" type="button" onclick={(e) => open(moreItems, e)}>{t('assist.more')}<Icon name="chevron-down" size={13} /></button>
      {/if}
    {/if}
    <button class="btn sm icon" type="button" title={t('term.files')} aria-label={t('term.files')} onclick={() => native.openWindow('files', serverId, name, nativeWindows)}><Icon name="folder" size={14} /></button>
    <button class="btn sm icon" type="button" title={t('term.manager')} aria-label={t('term.manager')} onclick={() => native.openWindow('manager', serverId, name, nativeWindows)}><Icon name="activity" size={14} /></button>
    {#if onwindow}<button class="btn sm icon" type="button" title={t('pult.inWindow')} aria-label={t('pult.inWindow')} onclick={onwindow}><Icon name="external-link" size={14} /></button>{/if}
  </div>
  {#if !online}<p class="dim pad">{t('assist.offline')}</p>
  {:else}
    <div class="asst-box">
      <div class="asst-screen"><AssistantSessionView {serverId} {session} bind:phase bind:info bind:this={screen} /></div>
      {#if $assistInfoShown}<AssistantStats {serverId} {phase} {info} onsettings={() => (quality = true)} />{/if}
    </div>
  {/if}
</div>
{#if menu}<ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />{/if}
{#if quality}<AssistantQuality {serverId} onclose={() => (quality = false)} />{/if}
