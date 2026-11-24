<script lang="ts">
  import ContextMenu, { type CtxItem } from '../ContextMenu.svelte';
  import { t } from '../i18n.svelte';
  import FloatingBar from '../FloatingBar.svelte';
  import { createFullscreen } from '../fullscreen.svelte';
  import Icon from '../Icon.svelte';
  import { native } from '../native';
  import DesktopSessionView, { type DeskPhase } from './DesktopSession.svelte';

  // The remote desktop (RDP) of a Windows computer: the bar, then the client. Used in a Console tab and on the server page.
  let { serverId, name, address, online, nativeWindows = false, onwindow }: { serverId: string; name: string; address: string; online: boolean; nativeWindows?: boolean; onwindow?: () => void } = $props();

  let phase = $state<DeskPhase>('form');
  let autoResize = $state(true);
  /** The scale in force (a new connection starts as "fit"). */
  let scale = $state<'fit' | 'real'>('fit');
  let session = $state<DesktopSessionView | null>(null);
  let menu = $state<{ x: number; y: number; items: CtxItem[] } | null>(null);
  let root = $state<HTMLDivElement | null>(null);
  const full = createFullscreen({ native: () => nativeWindows, root: () => root });

  const live = $derived(phase === 'open');
  const statusText = $derived(phase === 'connecting' ? t('desk.connecting') : phase === 'open' ? t('desk.open') : t('desk.closed'));
  $effect(() => {
    if (phase !== 'open') scale = 'fit';
  });
  // the button "display": the scale of the picture, the resolution that follows the window and the full screen
  const displayItems = $derived<CtxItem[]>([
    { key: 'fit', label: `${scale === 'fit' ? '✓ ' : ''}${t('desk.scaleFit')}`, onselect: () => { scale = 'fit'; session?.setScale('fit'); } },
    { key: 'real', label: `${scale === 'real' ? '✓ ' : ''}${t('desk.scaleReal')}`, onselect: () => { scale = 'real'; session?.setScale('real'); } },
    { key: 'auto', label: `${autoResize ? '✓ ' : ''}${t('desk.autoResize')}`, onselect: () => (autoResize = !autoResize) },
    { key: 'full', label: full.on ? t('fs.exit') : t('desk.fullscreen'), separated: true, onselect: () => void full.toggle() },
  ]);
  const moreItems = $derived.by<CtxItem[]>(() => {
    const s = session;
    const clip = s?.clipboardAvailable() ?? false;
    return [
      { key: 'tm', label: t('desk.taskManager'), onselect: () => s?.combo(['ControlLeft', 'ShiftLeft', 'Escape']) },
      { key: 'win', label: t('desk.winKey'), onselect: () => s?.winKey() },
      { key: 'lock', label: t('desk.lock'), onselect: () => s?.combo(['MetaLeft', 'KeyL']) },
      { key: 'run', label: t('desk.run'), onselect: () => s?.combo(['MetaLeft', 'KeyR']) },
      { key: 'alttab', label: t('desk.altTab'), onselect: () => s?.combo(['AltLeft', 'Tab']) },
      { key: 'send', label: t('desk.clipSend'), separated: true, disabled: !clip, onselect: () => void s?.sendClipboard() },
      { key: 'save', label: t('desk.clipSave'), disabled: !clip, onselect: () => void s?.saveClipboard() },
      { key: 'off', label: t('desk.disconnect'), separated: true, onselect: () => s?.disconnect() },
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
