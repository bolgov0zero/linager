<script lang="ts">
  import { untrack } from 'svelte';
  import { serverApi2, type F2bSettings } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { errorText } from '../errors';
  import { durationText, isIPv4, isIPv6, joinBan, matchPreset, PRESETS, previewConf, splitBan, type Preset, type Unit } from '../f2b-util';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';

  let { serverId, settings, selfIp, onsaved, onclose }: { serverId: string; settings: F2bSettings; selfIp: string; onsaved: () => Promise<void>; onclose: () => void } = $props();

  const LOOPBACK = ['127.0.0.0/8', '127.0.0.1/8', '127.0.0.1', '::1'];
  // the form starts from the current settings and is edited locally
  const init = untrack(() => settings);
  const start = splitBan(init.bantime);
  let maxretry = $state(init.maxretry);
  let findMin = $state(Math.max(1, Math.round(init.findtime / 60)));
  let banValue = $state(start.value);
  let banUnit = $state<Unit>(start.unit);
  let increment = $state(init.increment);
  let recidive = $state(init.recidive);
  let ignore = $state<string[]>(init.ignore.filter((x) => !LOOPBACK.includes(x) && x !== selfIp));
  let newIgnore = $state('');
  let ignoreError = $state('');
  let busy = $state(false);
  let error = $state('');

  const bantime = $derived(joinBan(banValue, banUnit));
  const draft = $derived<F2bSettings>({ maxretry, findtime: findMin * 60, bantime, increment, recidive, ignore });
  const active = $derived(matchPreset(draft));
  const valid = $derived(Number.isInteger(maxretry) && maxretry >= 1 && maxretry <= 100 && Number.isInteger(findMin) && findMin >= 1 && findMin <= 1440 && (banUnit === 'perm' || (Number.isInteger(banValue) && banValue >= 1)));
  const summary = $derived(t('sec.f2b.summaryLong', { n: maxretry, min: findMin, ban: durationText(bantime, t) }));
  const units = $derived([{ v: 'm', t: t('sec.f2b.unit.m') }, { v: 'h', t: t('sec.f2b.unit.h') }, { v: 'd', t: t('sec.f2b.unit.d') }, { v: 'perm', t: t('sec.f2b.unit.perm') }]);

  function applyPreset(p: Preset): void {
    const v = PRESETS[p];
    maxretry = v.maxretry;
    findMin = v.findtime / 60;
    const b = splitBan(v.bantime);
    banValue = b.value;
    banUnit = b.unit;
  }
  function addIgnore(): void {
    const v = newIgnore.trim();
    if (!v) return;
    const [addr, prefix] = v.split('/');
    const ok = (isIPv4(addr) || isIPv6(addr)) && (prefix === undefined || (/^\d{1,3}$/.test(prefix) && Number(prefix) <= (addr.includes(':') ? 128 : 32)));
    if (!ok) {
      ignoreError = t('sec.f2b.ignore.bad');
      return;
    }
    ignoreError = '';
    if (!ignore.includes(v) && v !== selfIp) ignore = [...ignore, v];
    newIgnore = '';
  }
  async function save(): Promise<void> {
    if (!valid || busy) return;
    busy = true;
    error = '';
    try {
      await serverApi2.securityF2bSettings(serverId, draft);
      await onsaved();
      onclose();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t('sec.f2b.settingsTitle')} {onclose}>
  <div class="sh">{t('sec.f2b.threshold')}</div>
  <div class="row">
    {#each ['soft', 'norm', 'hard'] as p (p)}<button class="chip" type="button" aria-pressed={active === p} onclick={() => applyPreset(p as Preset)}>{t(`sec.f2b.preset.${p}`)}</button>{/each}
    <span class="dim">{t('sec.f2b.orManual')}</span>
  </div>
  <div class="three">
    <label class="fl"><span>{t('sec.f2b.maxretry')}</span><span class="field"><input type="number" min="1" max="100" bind:value={maxretry} /></span></label>
    <label class="fl"><span>{t('sec.f2b.findtime')}</span><span class="field"><input type="number" min="1" max="1440" bind:value={findMin} /></span></label>
    <div class="fl"><span>{t('sec.f2b.bantime')}</span>
      <div class="row">{#if banUnit !== 'perm'}<span class="field" style="width:70px"><input type="number" min="1" aria-label={t('sec.f2b.bantime')} bind:value={banValue} /></span>{/if}<CustomSelect value={banUnit} options={units} onchange={(v) => (banUnit = v as Unit)} /></div>
    </div>
  </div>
  <p class="dim">{summary}</p>
  <div class="sh">{t('sec.f2b.repeat')}</div>
  <label class="ck"><input type="checkbox" bind:checked={increment} />{t('sec.f2b.increment')}</label>
  <label class="ck"><input type="checkbox" bind:checked={recidive} />{t('sec.f2b.recidive')}</label>
  <div class="sh">{t('sec.f2b.ignore')}</div>
  <div class="ipchips">
    <span class="gtag">127.0.0.0/8</span>
    {#if selfIp}<span class="gtag">{selfIp} · {t('sec.f2b.yourAddress')}</span>{/if}
    {#each ignore as ip (ip)}<span class="gtag">{ip}<button type="button" class="x" aria-label={t('sec.f2b.ignore.remove', { ip })} onclick={() => (ignore = ignore.filter((x) => x !== ip))}><Icon name="x" size={11} /></button></span>{/each}
  </div>
  <form class="row" onsubmit={(e) => { e.preventDefault(); addIgnore(); }}>
    <span class="field grow"><input class="mono" placeholder={t('sec.f2b.ignore.placeholder')} aria-label={t('sec.f2b.ignore')} spellcheck="false" autocomplete="off" bind:value={newIgnore} oninput={() => (ignoreError = '')} /></span>
    <button class="btn" type="submit" disabled={!newIgnore.trim()}>{t('sec.f2b.ignore.add')}</button>
  </form>
  {#if ignoreError}<div class="err">{ignoreError}</div>{/if}
  <p class="dim">{t('sec.f2b.ignore.note')}</p>
  <details><summary class="dim">{t('sec.f2b.showConf')}</summary><pre class="pv">{previewConf(draft, selfIp)}</pre></details>
  {#if error}<div class="err" role="alert">{error}</div>{/if}
  <p class="dim">{t('sec.f2b.saveNote')}</p>
  {#snippet foot()}
    <span class="spacer"></span>
    <button class="btn" type="button" disabled={busy} onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" disabled={busy || !valid} onclick={save}>{t('sec.f2b.save')}</button>
  {/snippet}
</Modal>
