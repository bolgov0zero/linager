<script lang="ts">
  import { ApiError, assistantApi, type AssistSettings } from '../api';
  import { i18n, t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import { COLORS, PRESET_IDS, PRESETS, estimateMbit, presetOf } from './quality';
  import { setAssistSettings } from './settings';

  // The picture settings of the Assistant of one computer: presets, colours, frame rate, traffic limit, pointer.
  let { serverId, onclose }: { serverId: string; onclose: () => void } = $props();

  let draft = $state<AssistSettings>({ ...PRESETS.normal });
  let loaded = $state(false);
  let loadError = $state('');
  let saving = $state(false);
  let saveError = $state('');
  /** The number of open sessions that took the saved values; null while there are edits. */
  let applied = $state<number | null>(null);
  const preset = $derived(presetOf(draft));
  const estimate = $derived(estimateMbit(draft));
  const num = (n: number, digits = 1): string => n.toLocaleString(i18n.locale === 'ru' ? 'ru-RU' : 'en-GB', { maximumFractionDigits: digits });

  async function loadSettings(): Promise<void> {
    try {
      const got = await assistantApi.settings(serverId);
      draft = { ...got, show_cursor: got.show_cursor === true };
      loadError = '';
      loaded = true;
    } catch (e) {
      loadError = e instanceof ApiError && e.code ? e.code : t('common.genericError');
    }
  }
  $effect(() => {
    void serverId;
    loaded = false;
    void loadSettings();
  });
  function edit(patch: Partial<AssistSettings>): void {
    draft = { ...draft, ...patch };
    applied = null;
    saveError = '';
  }
  async function save(): Promise<void> {
    saving = true;
    saveError = '';
    try {
      const res = await assistantApi.saveSettings(serverId, draft);
      draft = { ...res.settings, show_cursor: res.settings.show_cursor === true };
      applied = res.applied;
      setAssistSettings(serverId, draft);
    } catch (e) {
      saveError = e instanceof ApiError && e.code ? e.code : t('common.genericError');
    } finally {
      saving = false;
    }
  }
</script>

<Modal title={t('assist.quality')} {onclose}>
  {#if loadError}<div class="err" role="alert">{loadError}</div><button class="btn sm" type="button" onclick={loadSettings}>{t('net.retry')}</button>
  {:else if !loaded}<p class="dim">{t('dash.loading')}</p>
  {:else}
    <div class="fl"><span>{t('manage.presets')}</span><small class="dim">{t('manage.presetsHint')}</small>
      <div class="row">{#each PRESET_IDS as id (id)}<button class="chip" type="button" aria-pressed={preset === id} onclick={() => edit({ ...PRESETS[id], show_cursor: draft.show_cursor })}>{t(`manage.preset.${id}`)}</button>{/each}
        <button class="chip" type="button" aria-pressed={preset === 'custom'} disabled>{t('manage.preset.custom')}</button></div></div>
    <div class="fl"><span>{t('manage.colors')}</span><small class="dim">{t('manage.colorsHint')}</small>
      <div class="row">{#each COLORS as c (c)}<button class="chip" type="button" aria-pressed={draft.colors === c} onclick={() => edit({ colors: c })}>{t(`manage.color.${c}`)}</button>{/each}</div></div>
    <div class="fl"><span>{t('manage.fps')}</span><small class="dim">{t('manage.fpsHint')}</small>
      <div class="row"><input class="range" type="range" min="1" max="30" step="1" value={draft.fps} aria-label={t('manage.fps')} oninput={(e) => edit({ fps: Number(e.currentTarget.value) })} /><output class="mono">{t('manage.fpsValue', { n: draft.fps })}</output></div></div>
    <div class="fl"><span>{t('manage.limit')}</span><small class="dim">{t('manage.limitHint')}</small>
      <div class="row"><input class="range" type="range" min="0.5" max="20" step="0.5" value={draft.mbit} disabled={draft.nolimit} aria-label={t('manage.limit')} oninput={(e) => edit({ mbit: Number(e.currentTarget.value) })} /><output class="mono">{draft.nolimit ? t('manage.noLimitValue') : t('assist.mbitLimit', { v: num(draft.mbit) })}</output></div>
      <label class="ck"><input type="checkbox" checked={draft.nolimit} onchange={(e) => edit({ nolimit: e.currentTarget.checked })} />{t('manage.noLimit')}</label></div>
    <div class="fl"><span>{t('manage.quality')}</span><small class="dim">{t('manage.qualityHint')}</small>
      <label class="ck"><input type="checkbox" checked={draft.auto} onchange={(e) => edit({ auto: e.currentTarget.checked })} />{t('manage.auto')}</label></div>
    <div class="fl"><span>{t('manage.cursor')}</span><small class="dim">{t('manage.cursorSub')}</small>
      <label class="ck"><input type="checkbox" checked={draft.show_cursor} onchange={(e) => edit({ show_cursor: e.currentTarget.checked })} />{t('manage.cursorShow')}</label>
      <p class="dim">{t('manage.cursorHint')}</p></div>
    <p class="dim">{t('manage.estimate', { peak: num(estimate), idle: num(0.05, 2) })}</p>
    {#if saveError}<div class="err" role="alert">{saveError}</div>{/if}
    {#if applied !== null}<div class="ok-t" role="status">{t('manage.saved', { n: applied })}</div>{/if}
  {/if}
  {#snippet foot()}
    <button class="btn" type="button" disabled={saving || !loaded} onclick={() => edit(PRESETS.normal)}>{t('manage.reset')}</button><span class="spacer"></span>
    <button class="btn" type="button" onclick={onclose}>{t('common.close')}</button>
    <button class="btn primary" type="button" disabled={saving || !loaded} onclick={save}>{t('common.save')}</button>
  {/snippet}
</Modal>
