<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi, type ServerDetail } from '../api';
  import DesktopPane from '../desktop/DesktopPane.svelte';
  import { t } from '../i18n.svelte';

  // The remote desktop in a window of its own.
  let { id, title, nativeWindows = false }: { id: string; title: string; nativeWindows?: boolean } = $props();
  let s = $state<ServerDetail | null>(null);
  // The record is read again every 20 seconds and replaced as a whole. The pane gets the id of the window, not `s.id`: a value
  // read from `s` would restart the connection (an effect that reads it runs again when `s` is replaced, equal value or not).

  async function load(): Promise<void> {
    s = await serverApi.detail(id).catch(() => s);
  }
  onMount(() => {
    document.title = `${t('win.rdp')} · ${title}`;
    void load();
    const i = setInterval(() => void load(), 20_000);
    return () => clearInterval(i);
  });
</script>

<div class="win-full win-pane">
  {#if s}<DesktopPane serverId={id} name={s.name} address={s.address} online={s.online} {nativeWindows} />{:else}<p class="dim pad">{t('dash.loading')}</p>{/if}
</div>
