<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Group } from '../api';
  import { isUnauthorized } from '../errors';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import ConfirmTab from './ConfirmTab.svelte';
  import LinkTab from './LinkTab.svelte';
  import SshTab from './SshTab.svelte';

  // Add a server: approve the agents that ask to join, make an install link, or install over SSH. The three tabs stay
  // mounted, so a link that was just made and a running install survive a switch of tabs.
  let { host, onclose, onadded, onopen, onauthlost }: {
    /** The address of the Linager server the application is connected to (without the port). */
    host: string;
    onclose: () => void;
    /** A server was approved, put on the dashboard or installed: the dashboard reloads. */
    onadded: () => void;
    /** Open the page of a server (by its name). */
    onopen: (name: string) => void;
    onauthlost: () => void;
  } = $props();

  type Tab = 'confirm' | 'link' | 'ssh';
  const TABS: Tab[] = ['confirm', 'link', 'ssh'];
  let tab = $state<Tab>('confirm');
  let pending = $state(0);
  let groups = $state<Group[]>([]);

  onMount(() => {
    void api.groups().then((g) => (groups = g)).catch((e) => {
      if (isUnauthorized(e)) onauthlost();
    });
  });
</script>

<Modal title={t('asv.title')} wide {onclose}>
  <div class="asv">
    <nav class="asv-nav" aria-label={t('asv.nav')}>
      {#each TABS as id (id)}
        <button type="button" aria-current={tab === id} onclick={() => (tab = id)}><span>{t(`asv.tab.${id}`)}</span>{#if id === 'confirm'}<em class:hot={pending > 0}>{pending}</em>{/if}</button>
      {/each}
    </nav>
    <div class="asv-body">
      <div hidden={tab !== 'confirm'}><ConfirmTab active={tab === 'confirm'} {host} bind:count={pending} {onadded} {onauthlost} /></div>
      <div hidden={tab !== 'link'}><LinkTab active={tab === 'link'} {groups} {onauthlost} /></div>
      <div hidden={tab !== 'ssh'}><SshTab {groups} {onadded} {onauthlost} onopen={(name) => { onclose(); onopen(name); }} /></div>
    </div>
  </div>
</Modal>
