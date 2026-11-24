<script lang="ts">
  import { t } from '../i18n.svelte';
  import { fmtBytes } from '../status';
  import CustomSelect from '../CustomSelect.svelte';
  import { autoCheck, checkUpdate, installUpdate, setAutoCheck, setChannel, startAutoCheck, upd, updateBusy } from '../update.svelte';
  import { onMount } from 'svelte';

  let auto = $state(autoCheck());
  const info = $derived(upd.info);
  const channels = $derived([{ v: 'stable', t: t('set.upd.ch.stable') }, { v: 'beta', t: t('set.upd.ch.beta') }]);
  function pickChannel(v: string): void {
    setChannel(v as 'stable' | 'beta');
    if (upd.info) upd.info = { ...upd.info, channel: v, newer: false, latest: undefined };
    void checkUpdate();
  }
  const phaseText = $derived(
    upd.status.phase === 'downloading' ? t('set.upd.downloading', { pct: upd.status.pct }) : upd.status.phase === 'installing' ? t('set.upd.installing') : upd.status.phase === 'restarting' ? t('set.upd.restarting') : '',
  );
  const reason = $derived(info?.reason === 'headless' ? t('set.upd.headless') : info?.reason === 'dev' ? t('set.upd.dev') : '');
  // this window has its own page: it reads what is known and asks again
  onMount(() => {
    const stop = startAutoCheck();
    return stop;
  });
</script>

<div class="set-sec">
  <h3>{t('set.upd.title')}</h3>
  <dl class="facts">
    <div><dt>{t('set.upd.current')}</dt><dd class="mono">{info?.current ?? '—'}</dd></div>
    <div><dt>{t('set.upd.channel')}</dt><dd>{#if info?.supported}<CustomSelect small value={info.channel} options={channels} onchange={pickChannel} />{:else}{info ? t(`set.upd.ch.${info.channel}`) : '—'}{/if}</dd></div>
  </dl>
  {#if reason}<p class="dim">{reason}</p>{/if}
  {#if info?.supported}
    {#if info.newer}
      <div class="upd-new">
        <div class="row"><b>{t('set.upd.available', { v: info.latest ?? '' })}</b>{#if info.size}<span class="dim">{fmtBytes(info.size)}</span>{/if}</div>
        {#if info.notes?.length}<ul class="upd-notes">{#each info.notes as n (n)}<li>{n}</li>{/each}</ul>{/if}
        {#if updateBusy()}
          <div class="row"><span class="bar lv-ok"><i style="width:{upd.status.pct}%"></i></span><span class="dim">{phaseText}</span></div>
        {:else}
          <div class="row"><button class="btn primary" type="button" onclick={installUpdate}>{t('set.upd.install')}</button></div>
        {/if}
        {#if upd.status.phase === 'error'}<div class="err" role="alert">{t('set.upd.failed')}: {upd.status.error}</div>{/if}
      </div>
    {:else if info.checked_at}
      <p class="ok-t">{t('set.upd.latest')}</p>
    {/if}
    {#if upd.failed}<div class="err" role="alert">{t('set.upd.checkFailed')}</div>{/if}
    <div class="row">
      <button class="btn" type="button" disabled={upd.checking || updateBusy()} onclick={checkUpdate}>{upd.checking ? t('set.upd.checking') : t('set.upd.check')}</button>
      <span class="grow"></span>
      <label class="ck"><input type="checkbox" bind:checked={auto} onchange={() => setAutoCheck(auto)} />{t('set.upd.auto')}</label>
    </div>
    <p class="dim">{t('set.upd.channelHint')}</p>
    <p class="dim">{t('set.upd.hint')}</p>
  {/if}
</div>
