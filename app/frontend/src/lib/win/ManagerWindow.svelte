<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi, type ServerDetail } from '../api';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import ProcessesTab from '../server/ProcessesTab.svelte';
  import ServicesTab from '../server/ServicesTab.svelte';
  import SessionsTab from '../server/SessionsTab.svelte';

  let { id, title }: { id: string; title: string } = $props();
  let s = $state<ServerDetail | null>(null);
  let tab = $state<'processes' | 'services' | 'users'>('processes');

  onMount(async () => {
    document.title = `${t('term.manager')} · ${title}`;
    s = await serverApi.detail(id).catch(() => null);
  });
</script>

<div class="win-full mgr">
  <div class="tbar">
    <button class="btn sm" type="button" aria-pressed={tab === 'processes'} onclick={() => (tab = 'processes')}><Icon name="cpu" size={14} />{t('sp.processes')}</button>
    <button class="btn sm" type="button" aria-pressed={tab === 'services'} onclick={() => (tab = 'services')}><Icon name="list-checks" size={14} />{t('sp.services')}</button>
    <button class="btn sm" type="button" aria-pressed={tab === 'users'} onclick={() => (tab = 'users')}><Icon name="users" size={14} />{t('mgr.users')}</button>
  </div>
  <div class="mgr-body">
    {#if s}{#key tab}{#if tab === 'processes'}<ProcessesTab {s} />{:else if tab === 'services'}<ServicesTab {s} />{:else}<SessionsTab {s} />{/if}{/key}{/if}
  </div>
</div>
