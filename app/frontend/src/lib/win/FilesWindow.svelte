<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi } from '../api';
  import { t } from '../i18n.svelte';
  import { isWindows } from '../status';
  import FilesView from '../server/FilesView.svelte';

  let { id, title, nativeWindows, start = '' }: { id: string; title: string; nativeWindows: boolean; start?: string } = $props();
  // the first folder: the one asked for, else the home of root (the agent works as root), on Windows the drive C:
  let first = $state<string | null>(null);
  onMount(async () => {
    document.title = `${t('sp.files')} · ${title}`;
    if (start) {
      first = start;
      return;
    }
    first = await serverApi.detail(id).then((s) => (isWindows(s.os) ? '/C:/' : '/root')).catch(() => '/root');
  });
</script>

<div class="win-full">{#if first !== null}<FilesView {id} {title} {nativeWindows} start={first} win />{/if}</div>
