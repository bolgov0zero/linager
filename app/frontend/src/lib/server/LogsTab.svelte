<script lang="ts">
  import { tick } from 'svelte';
  import { openLogs, type ServerDetail } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import Icon from '../Icon.svelte';
  import SearchField from '../SearchField.svelte';
  import { t } from '../i18n.svelte';

  let { s }: { s: ServerDetail } = $props();

  let unit = $state('');
  let priority = $state('6');
  let kernel = $state(false);
  let text = $state('');
  let status = $state<'connecting' | 'open' | 'closed'>('connecting');
  let box = $state<HTMLDivElement | null>(null);
  // follow the end of the log only while the reader is at the end; a scroll up stops it
  let stick = true;
  const atEnd = (): boolean => !box || box.scrollHeight - box.scrollTop - box.clientHeight < 24;
  const MAX = 200_000;

  const prios = $derived([
    { v: '7', t: t('sp.log.p7') }, { v: '6', t: t('sp.log.p6') }, { v: '4', t: t('sp.log.p4') }, { v: '3', t: t('sp.log.p3') },
  ]);

  // the server record is replaced on every refresh: only a changed id or state may restart the stream
  const sid = $derived(s.id);
  const online = $derived(s.online);

  $effect(() => {
    const u = unit.trim(), p = priority, k = kernel, id = sid;
    if (!online) return;
    text = '';
    stick = true;
    status = 'connecting';
    const ws = openLogs(id, { unit: u || undefined, priority: Number(p), kernel: k, lines: 200 });
    const dec = new TextDecoder();
    ws.onopen = () => (status = 'open');
    ws.onmessage = (e: MessageEvent<unknown>) => {
      if (typeof e.data === 'string') return; // an error or the end of the stream
      text += dec.decode(new Uint8Array(e.data as ArrayBuffer), { stream: true });
      if (text.length > MAX) text = text.slice(text.length - MAX / 2);
      if (stick) void tick().then(() => box && (box.scrollTop = box.scrollHeight));
    };
    ws.onclose = () => (status = 'closed');
    return () => ws.close();
  });
</script>

<div class="sp-fill">
<div class="sp-tools">
  <SearchField placeholder={t('sp.log.unit')} bind:value={unit} spellcheck={false} />
  <CustomSelect value={priority} options={prios} onchange={(v) => (priority = v)} />
  <button class="chip" type="button" aria-pressed={kernel} onclick={() => (kernel = !kernel)}>{t('sp.log.kernel')}</button>
  <span class="grow"></span><span class="dim">{status === 'open' ? 'journalctl' : status === 'connecting' ? t('term.connecting') : t('term.closed')}</span>
</div>
{#if !online}<p class="dim">{t('sp.offline')}</p>{:else}<div class="term logv" bind:this={box} onscroll={() => (stick = atEnd())}>{text}</div>{/if}
</div>
