<script lang="ts">
  import { i18n, t } from '../i18n.svelte';
  import AssistantBadge from './AssistantBadge.svelte';
  import type { AssistInfo, AssistPhase } from './AssistantSession.svelte';
  import { assistSettings, refreshAssistSettings } from './settings';

  /** The line under the screen: what really happens (resolution, colours, frames, traffic) and the way to the settings. */
  let { serverId, phase, info, onsettings }: { serverId: string; phase: AssistPhase; info: AssistInfo | null; onsettings?: () => void } = $props();

  const settings = $derived($assistSettings[serverId] ?? null);
  $effect(() => {
    if (phase !== 'open') return;
    void serverId;
    void refreshAssistSettings(serverId);
    const timer = setInterval(() => void refreshAssistSettings(serverId), 20000);
    return () => clearInterval(timer);
  });
  const live = $derived(phase === 'open' && info !== null);
  const num = (n: number, digits = 1): string => n.toLocaleString(i18n.locale === 'ru' ? 'ru-RU' : 'en-GB', { maximumFractionDigits: digits });
  const limit = $derived(settings ? (settings.nolimit ? t('assist.noLimit') : t('assist.mbitLimit', { v: num(settings.mbit) })) : '');
</script>

<div class="asst-stat">
  <span>{live && info ? `${info.w}×${info.h}` : '—'}</span>
  {#if live}<AssistantBadge desktop={info?.desktop} />{/if}
  {#if settings}<span>{t('assist.stat.colors')} <b>{t(`assist.colorNote.${settings.colors}`)}</b></span>{/if}
  <span>{t('assist.stat.frames')} <b>{live && info ? Math.round(info.fps) : 0}{settings ? ` ${t('assist.stat.of')} ${settings.fps}` : ''}</b></span>
  <span>{t('assist.stat.traffic')} <b>{live && info ? num(info.kbps / 1000) : '0'} {t('assist.mbit')}</b>{limit ? ` (${t('assist.stat.limit')} ${limit})` : ''}</span>
  <span class="grow"></span>
  {#if onsettings}<button class="btn xs" type="button" style="width:auto;padding:0 8px" onclick={onsettings}>{t('assist.configure')}</button>{/if}
</div>
