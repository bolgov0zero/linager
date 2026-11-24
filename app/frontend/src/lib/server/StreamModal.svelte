<script lang="ts">
  import { onMount } from 'svelte';
  import { openStream } from '../api';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';

  // A job of the agent with a live output (an install, a pull): the text as it comes, and the end.
  let { title, serverId, kind, params = {}, onend, onclose }: {
    title: string; serverId: string; kind: string; params?: Record<string, string | number>;
    /** Called once when the stream is over; failed is true after an error. */
    onend?: (failed: boolean) => void; onclose: () => void;
  } = $props();

  let text = $state('');
  let phase = $state<'connecting' | 'live' | 'ended' | 'failed'>('connecting');
  let box = $state<HTMLDivElement | null>(null);
  // follow the end only while the reader is at the end
  let stick = true;

  onMount(() => {
    const ws = openStream(serverId, kind, params);
    const dec = new TextDecoder();
    let failed = false;
    let done = false;
    const finish = (): void => {
      if (done) return;
      done = true;
      phase = failed ? 'failed' : 'ended';
      onend?.(failed);
    };
    ws.onopen = () => (phase = 'live');
    ws.onmessage = (e: MessageEvent<unknown>) => {
      if (typeof e.data === 'string') {
        try {
          const f = JSON.parse(e.data) as { closed?: unknown; error?: unknown };
          if (f.error !== undefined) {
            failed = true;
            text += `\n${String(f.error)}\n`;
          }
          if (f.closed !== undefined) {
            finish();
            ws.close();
          }
        } catch {
          text += e.data;
        }
        return;
      }
      text += dec.decode(new Uint8Array(e.data as ArrayBuffer), { stream: true });
      if (text.length > 200_000) text = text.slice(-100_000);
      if (stick) queueMicrotask(() => box && (box.scrollTop = box.scrollHeight));
    };
    ws.onerror = () => (failed = true);
    ws.onclose = finish;
    return () => ws.close();
  });
</script>

<Modal {title} wide {onclose}>
  <p class="dim">{t(`docker.stream.${phase}`)}</p>
  <div class="term logv stream-log" bind:this={box} role="log" onscroll={() => (stick = !box || box.scrollHeight - box.scrollTop - box.clientHeight < 24)}>{text || t('docker.pull.waiting')}</div>
  {#snippet foot()}
    <span class="spacer"></span>
    <button class="btn" type="button" onclick={onclose}>{t('common.close')}</button>
  {/snippet}
</Modal>
