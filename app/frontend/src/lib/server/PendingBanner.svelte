<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi2, type PendingChange } from '../api';
  import { errorText } from '../errors';
  import { t } from '../i18n.svelte';

  // A change with the rollback timer: the agent has applied it and rolls it back by itself when it is not confirmed in
  // time (the connection may be lost by the change). Used by the network and the firewall tabs.
  let { serverId, pending, lost = false, onconfirmed, onchanged }: {
    serverId: string; pending: PendingChange; lost?: boolean; onconfirmed: (id: string) => void; onchanged: () => Promise<void>;
  } = $props();

  let now = $state(Date.now());
  let busy = $state(false);
  let error = $state('');
  const left = $derived(Math.max(0, Math.ceil(pending.deadline - now / 1000)));
  const expired = $derived(pending.deadline - now / 1000 <= 0);
  const fw = $derived(pending.kind === 'firewall');

  onMount(() => {
    const i = setInterval(() => (now = Date.now()), 250);
    return () => clearInterval(i);
  });

  async function act(kind: 'confirm' | 'rollback'): Promise<void> {
    if (busy) return;
    busy = true;
    error = '';
    try {
      if (kind === 'confirm') {
        await serverApi2.changeConfirm(serverId, pending.kind, pending.id);
        onconfirmed(pending.id);
      } else await serverApi2.changeRollback(serverId, pending.kind, pending.id);
      await onchanged();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<section class="pending" aria-labelledby="pend-t">
  <div class="count"><b>{left}</b><span>{t('net.pending.seconds')}</span></div>
  <div class="pend-body">
    <h4 id="pend-t">{t(fw ? 'fw.pending.title' : 'net.pending.title')}</h4>
    <p>{lost ? t(fw ? 'fw.pending.lost' : 'net.pending.lost', { n: left }) : expired ? t(fw ? 'fw.pending.expired' : 'net.pending.expired') : t(fw ? 'fw.pending.explain' : 'net.pending.text')}</p>
    {#if error}<div class="err" role="alert">{error}</div>{/if}
    <div class="row">
      <button class="btn primary" type="button" disabled={busy || lost || expired} onclick={() => act('confirm')}>{t(fw ? 'fw.pending.confirm' : 'net.pending.confirm')}</button>
      <button class="btn danger" type="button" disabled={busy || lost || expired} onclick={() => act('rollback')}>{t(fw ? 'fw.pending.rollback' : 'net.pending.rollback')}</button>
    </div>
  </div>
</section>
