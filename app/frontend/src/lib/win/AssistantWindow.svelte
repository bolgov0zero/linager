<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi, type ServerDetail } from '../api';
  import AssistantPane from '../assistant/AssistantPane.svelte';
  import { t } from '../i18n.svelte';

  // The Assistant in a window of its own.
  let { id, title, nativeWindows = false }: { id: string; title: string; nativeWindows?: boolean } = $props();
  let s = $state<ServerDetail | null>(null);

  async function load(): Promise<void> {
    s = await serverApi.detail(id).catch(() => s);
  }
  onMount(() => {
    document.title = `${t('win.asst')} · ${title}`;
    void load();
    const i = setInterval(() => void load(), 20_000);
    return () => clearInterval(i);
  });
</script>

<div class="win-full win-pane">
  {#if s}<AssistantPane serverId={s.id} name={s.name} online={s.online} {nativeWindows} />{:else}<p class="dim pad">{t('dash.loading')}</p>{/if}
</div>
