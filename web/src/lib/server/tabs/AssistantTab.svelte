<script lang="ts">
  import { untrack } from 'svelte';
  import type { ServerDetail } from '../../../api';
  import { tr } from '../../../i18n';
  import AssistantBadge from '../../assistant/AssistantBadge.svelte';
  import AssistantSession, { type AssistInfo, type AssistPhase } from '../../assistant/AssistantSession.svelte';
  import { assistInfoShown } from '../../assistant/settings';
  import AssistantStats from '../../assistant/AssistantStats.svelte';
  import AssistantTools from '../../assistant/AssistantTools.svelte';
  import { takeAssistSession } from '../../assistant/nav';
  import { openAssistantWindow, openQualityWindow } from '../../terminal/windows';

  /** docked: the tab fills the Console (lib/pult): it takes the height of its parent and has no "in a window" button. */
  let { server, docked = false, paused = false }: { server: ServerDetail; docked?: boolean; /** The tab is hidden: the computer stops sending pictures. */ paused?: boolean } = $props();

  // Derived primitives: the parent replaces `server` on every live metrics event, which must not restart the session.
  const serverId = $derived(server.id);
  const serverName = $derived(server.name);

  let phase = $state<AssistPhase>('connecting');
  let info = $state<AssistInfo | null>(null);
  let screen = $state<AssistantSession | null>(null);
  // The Manage tab can open this tab with a session of its table.
  let session = $state(untrack(() => takeAssistSession(server.id)));

  $effect(() => {
    screen?.setPaused(paused);
  });

  // The picture fitted into the tab, or at 100% with scroll bars.
  let display = $state<'fit' | 'fill' | 'window' | 'max' | 'real'>('fit');
  $effect(() => {
    screen?.setScale(display === 'real' ? 'real' : display === 'fill' ? 'fill' : 'fit');
  });

  const statusText = $derived(
    phase === 'connecting' ? $tr('assist.connectingShort') : phase === 'open' ? $tr('assist.open') : $tr('assist.closed'),
  );

  /** Moves the Assistant into a floating window: this tab's own connection ends first (one window per session). */
  function popOut(): void {
    screen?.disconnect();
    setTimeout(() => openAssistantWindow(serverId, serverName, session), 150);
  }
</script>

<div class="asstTab" class:docked>
  <div class="head">
    <p class="meta">
      <span class="dot" class:on={server.online && phase === 'open'} aria-hidden="true"></span>
      <span class="hdr">{$tr('assist.header', { name: server.name })}</span>
      {#if server.online}<span class="muted">· {statusText}</span>{/if}
      {#if server.online && phase === 'open'}<AssistantBadge desktop={info?.desktop} />{/if}
    </p>
    {#if server.online}
      <div class="actions">
        <AssistantTools {serverId} bind:session {phase} {info} {screen} layout="tab" bind:display onsettings={() => openQualityWindow(serverId, serverName)} {serverName} />
        {#if !docked}<button class="btn sm" type="button" onclick={popOut}>{$tr('termwin.openWindow')}</button>{/if}
      </div>
    {/if}
  </div>
  {#if !server.online}
    <p class="note">{$tr('assist.offline')}</p>
  {:else}
    <div class="box">
      <div class="screenbox"><AssistantSession {serverId} {session} bind:phase bind:info bind:this={screen} /></div>
      {#if $assistInfoShown}
        <AssistantStats {serverId} {phase} {info} />
      {/if}
    </div>
  {/if}
</div>

<style>
  .asstTab {
    display: flex;
    flex-direction: column;
    gap: 9px;
    height: calc(100vh - 198px);
    min-height: 378px;
  }

  .asstTab.docked {
    height: 100%;
    min-height: 0;
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 9px;
    flex-wrap: wrap;
  }

  .meta {
    display: flex;
    align-items: center;
    gap: 7px;
    margin: 0;
  }

  .dot {
    width: 9px;
    height: 9px;
    background: var(--muted);
  }

  .dot.on {
    background: var(--good);
  }

  .actions {
    display: flex;
    gap: 7px;
    flex-wrap: wrap;
    align-items: center;
  }

  .box {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    border: 2px solid var(--line);
  }

  .screenbox {
    flex: 1;
    min-height: 0;
    display: flex;
  }

  .note {
    margin: 0;
  }
</style>
