<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type Session } from './api';
  import { t } from './i18n.svelte';

  // The device signed in, but an administrator has not approved it yet. The server lets it ask who it is and sign out, nothing else:
  // this screen asks again every few seconds and goes on as soon as the answer changes.
  let { name, onapproved, onlogout, onlost }: { name: string; onapproved: (s: Session) => void; onlogout: () => void; onlost: (why: 'denied' | 'ended') => void } = $props();

  const POLL_MS = 3000;
  const since = Date.now();
  let tick = $state(0);
  let failed = $state(false);

  async function poll(): Promise<void> {
    try {
      const s = await api.me();
      failed = false;
      if (s.device?.status === 'approved' || !s.device?.confirm) onapproved(s);
    } catch (e) {
      if (e instanceof ApiError && e.status === 403 && e.code === 'device_denied') onlost('denied');
      else if (e instanceof ApiError && e.status === 401) onlost('ended'); // removed or signed out by an administrator
      else failed = true;
    }
  }

  onMount(() => {
    void poll();
    const p = setInterval(() => void poll(), POLL_MS);
    const c = setInterval(() => (tick = Date.now()), 1000);
    return () => {
      clearInterval(p);
      clearInterval(c);
    };
  });

  const waited = $derived(Math.max(0, Math.floor(((tick || since) - since) / 1000)));
  const clock = $derived(`${Math.floor(waited / 60)}:${String(waited % 60).padStart(2, '0')}`);
</script>

<div class="center">
  <div class="panel-card" style="text-align:center">
    <div class="brand"><img class="logo" src="icon.svg" alt="" /><b>Linager</b></div>
    <h1>{t('wait.title')}</h1>
    <p class="muted" style="font-size:12.5px">{t('wait.text', { name })}</p>
    <p class="mono" style="font-size:20px;margin:6px 0" aria-live="off">{clock}</p>
    <p class="dim" style="font-size:12.5px">{t('wait.hint')}</p>
    {#if failed}<div class="err" role="alert">{t('wait.offline')}</div>{/if}
    <div class="row" style="justify-content:center"><button class="btn" type="button" onclick={onlogout}>{t('wait.logout')}</button></div>
  </div>
</div>
