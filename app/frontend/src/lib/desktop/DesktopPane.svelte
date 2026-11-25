<script lang="ts">
  import ContextMenu, { type CtxItem } from '../ContextMenu.svelte';
  import { t } from '../i18n.svelte';
  import FloatingBar from '../FloatingBar.svelte';
  import { createFullscreen } from '../fullscreen.svelte';
  import Icon from '../Icon.svelte';
  import { flagValue, parseFlag, parseRdpScale, rdpAutoKey, rdpScaleKey, type RdpScale } from '../display-pref';
  import { native } from '../native';
  import { getPref, setPref } from '../prefs';
  import DesktopSessionView, { type DeskPhase } from './DesktopSession.svelte';

  // The remote desktop (RDP) of a Windows computer: the bar, then the client. Used in a Console tab and on the server page.
  let { serverId, name, address, online, nativeWindows = false, onwindow }: { serverId: string; name: string; address: string; online: boolean; nativeWindows?: boolean; onwindow?: () => void } = $props();

  let phase = $state<DeskPhase>('form');
  // the scale and the resolution that follows the window are remembered for each server (by the application)
  // svelte-ignore state_referenced_locally
  let autoResize = $state(parseFlag(getPref(rdpAutoKey(serverId)), true));
  // svelte-ignore state_referenced_locally
  let scale = $state<RdpScale>(parseRdpScale(getPref(rdpScaleKey(serverId))));
  $effect(() => {
    autoResize = parseFlag(getPref(rdpAutoKey(serverId)), true);
    scale = parseRdpScale(getPref(rdpScaleKey(serverId)));
  });
  // a new connection starts as "fit": the remembered scale is put on it when it is open
  $effect(() => {
    if (phase === 'open') session?.setScale(scale);
  });
  function setScale(v: RdpScale): void {
    scale = v;
    setPref(rdpScaleKey(serverId), v);
    session?.setScale(v);
  }
  function toggleAuto(): void {
    autoResize = !autoResize;
    setPref(rdpAutoKey(serverId), flagValue(autoResize));
  }
  let session = $state<DesktopSessionView | null>(null);
  let menu = $state<{ x: number; y: number; items: CtxItem[] } | null>(null);
  let root = $state<HTMLDivElement | null>(null);
  const full = createFullscreen({ native: () => nativeWindows, root: () => root });

  const live = $derived(phase === 'open');
  const statusText = $derived(phase === 'connecting' ? t('desk.connecting') : phase === 'open' ? t('desk.open') : t('desk.closed'));
  // the button "display": the scale of the picture, the resolution that follows the window and the full screen
  const displayItems = $derived<CtxItem[]>([
    { key: 'fit', icon: 'scaling', label: t('desk.scaleFit'), selected: scale === 'fit', onselect: () => setScale('fit') },
    { key: 'real', icon: 'scan', label: t('desk.scaleReal'), selected: scale === 'real', onselect: () => setScale('real') },
    { key: 'auto', icon: 'refresh-cw', label: t('desk.autoResize'), selected: autoResize, separated: true, onselect: toggleAuto },
    { key: 'full', icon: full.on ? 'minimize-2' : 'maximize', label: full.on ? t('fs.exit') : t('desk.fullscreen'), separated: true, onselect: () => void full.toggle() },
  ]);
  const moreItems = $derived.by<CtxItem[]>(() => {
    const s = session;
    const clip = s?.clipboardAvailable() ?? false;
    return [
      { key: 'tm', icon: 'activity', label: t('desk.taskManager'), hint: 'Ctrl+Shift+Esc', onselect: () => s?.combo(['ControlLeft', 'ShiftLeft', 'Escape']) },
      { key: 'win', icon: 'layout-grid', label: t('desk.winKey'), hint: 'Win', onselect: () => s?.winKey() },
      { key: 'lock', icon: 'lock', label: t('desk.lock'), hint: 'Win+L', onselect: () => s?.combo(['MetaLeft', 'KeyL']) },
      { key: 'run', icon: 'play', label: t('desk.run'), hint: 'Win+R', onselect: () => s?.combo(['MetaLeft', 'KeyR']) },
      { key: 'alttab', icon: 'arrow-left-right', label: t('desk.altTab'), hint: 'Alt+Tab', onselect: () => s?.combo(['AltLeft', 'Tab']) },
      { key: 'send', icon: 'clipboard-paste', label: t('desk.clipSend'), separated: true, disabled: !clip, onselect: () => void s?.sendClipboard() },
      { key: 'save', icon: 'clipboard-copy', label: t('desk.clipSave'), disabled: !clip, onselect: () => void s?.saveClipboard() },
      { key: 'off', icon: 'unplug', label: t('desk.disconnect'), danger: true, separated: true, onselect: () => s?.disconnect() },
    ];
  });
  function open(items: CtxItem[], ev: MouseEvent): void {
    const r = (ev.currentTarget as HTMLElement).getBoundingClientRect();
    menu = { x: r.left, y: r.bottom + 2, items };
  }
</script>

{#snippet bar()}
    <span class="dot" class:ok={online && phase === 'open'}></span>
    <b>{t('desk.header', { name })}</b>{#if online}<span class="dim">· {statusText}</span>{/if}
    <span class="grow"></span>
    {#if online && live}
      <button class="btn sm icon" type="button" title={t('desk.display')} aria-label={t('desk.display')} onclick={(e) => open(displayItems, e)}><Icon name="monitor" size={14} /></button>
      <button class="btn sm icon" type="button" title={t('desk.cadHint')} aria-label="Ctrl+Alt+Del" onclick={() => session?.ctrlAltDel()}><Icon name="keyboard" size={14} /></button>
      <button class="btn sm" type="button" onclick={(e) => open(moreItems, e)}>{t('desk.more')}<Icon name="chevron-down" size={13} /></button>
    {/if}
    <button class="btn sm icon" type="button" title={t('term.files')} aria-label={t('term.files')} onclick={() => native.openWindow('files', serverId, name, nativeWindows)}><Icon name="folder" size={14} /></button>
    <button class="btn sm icon" type="button" title={t('term.manager')} aria-label={t('term.manager')} onclick={() => native.openWindow('manager', serverId, name, nativeWindows)}><Icon name="activity" size={14} /></button>
    {#if full.on}<button class="btn sm icon" type="button" title={t('fs.exit')} aria-label={t('fs.exit')} onclick={() => void full.set(false)}><Icon name="minimize-2" size={14} /></button>
    {:else if onwindow}<button class="btn sm icon" type="button" title={t('pult.inWindow')} aria-label={t('pult.inWindow')} onclick={onwindow}><Icon name="external-link" size={14} /></button>{/if}
{/snippet}

<div class="asst-pane" class:fs={full.on} bind:this={root}>
  {#if full.on}<FloatingBar>{@render bar()}</FloatingBar>{:else}<div class="asst-bar">{@render bar()}</div>{/if}
  {#if !online}<p class="dim pad">{t('desk.offline')}</p>
  {:else}<div class="asst-box"><div class="asst-screen"><DesktopSessionView {serverId} {address} bind:phase bind:autoResize bind:this={session} /></div></div>{/if}
</div>
{#if menu}<ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />{/if}
