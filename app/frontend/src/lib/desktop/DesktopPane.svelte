<script lang="ts">
  import ContextMenu, { type CtxItem } from '../ContextMenu.svelte';
  import { t } from '../i18n.svelte';
  import Icon from '../Icon.svelte';
  import DesktopSessionView, { type DeskPhase } from './DesktopSession.svelte';

  // The remote desktop (RDP) of a Windows computer: the bar, then the client. Used in a Console tab and on the server page.
  let { serverId, name, address, online }: { serverId: string; name: string; address: string; online: boolean } = $props();

  let phase = $state<DeskPhase>('form');
  let autoResize = $state(true);
  let session = $state<DesktopSessionView | null>(null);
  let menu = $state<{ x: number; y: number; items: CtxItem[] } | null>(null);

  const live = $derived(phase === 'open');
  const statusText = $derived(phase === 'connecting' ? t('desk.connecting') : phase === 'open' ? t('desk.open') : t('desk.closed'));
  const items = $derived.by<CtxItem[]>(() => {
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
      { key: 'fit', label: t('desk.scaleFit'), separated: true, onselect: () => s?.setScale('fit') },
      { key: 'real', label: t('desk.scaleReal'), onselect: () => s?.setScale('real') },
      { key: 'auto', label: `${autoResize ? '✓ ' : ''}${t('desk.autoResize')}`, onselect: () => (autoResize = !autoResize) },
      { key: 'full', label: t('desk.fullscreen'), onselect: () => s?.fullscreen() },
      { key: 'off', label: t('desk.disconnect'), separated: true, onselect: () => s?.disconnect() },
    ];
  });
  function more(ev: MouseEvent): void {
    const r = (ev.currentTarget as HTMLElement).getBoundingClientRect();
    menu = { x: r.left, y: r.bottom + 2, items };
  }
</script>

<div class="asst-pane">
  <div class="asst-bar">
    <span class="dot" class:ok={online && phase === 'open'}></span>
    <b>{t('desk.header', { name })}</b>{#if online}<span class="dim">· {statusText}</span>{/if}
    <span class="grow"></span>
    {#if online && live}
      <button class="btn sm" type="button" title={t('desk.cadHint')} onclick={() => session?.ctrlAltDel()}>Ctrl+Alt+Del</button>
      <button class="btn sm" type="button" onclick={more}>{t('desk.more')}<Icon name="chevron-down" size={13} /></button>
    {/if}
  </div>
  {#if !online}<p class="dim pad">{t('desk.offline')}</p>
  {:else}<div class="asst-box"><div class="asst-screen"><DesktopSessionView {serverId} {address} bind:phase bind:autoResize bind:this={session} /></div></div>{/if}
</div>
{#if menu}<ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />{/if}
