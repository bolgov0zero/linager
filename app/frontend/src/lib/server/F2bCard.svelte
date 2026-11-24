<script lang="ts">
  import { serverApi2, type SecState } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { errorText } from '../errors';
  import { durationText, PRESETS, type Preset } from '../f2b-util';
  import { t } from '../i18n.svelte';
  import F2bBansModal from './F2bBansModal.svelte';
  import F2bSettingsModal from './F2bSettingsModal.svelte';
  import StreamModal from './StreamModal.svelte';

  let { serverId, sec, unreachable, onchanged }: { serverId: string; sec: SecState; unreachable: boolean; onchanged: () => Promise<void> } = $props();

  const f2b = $derived(sec.fail2ban);
  const settings = $derived(f2b.settings);
  const bannedNow = $derived(f2b.bans.length);
  const totalBanned = $derived(f2b.jails.reduce((n, j) => n + j.total_banned, 0));
  const failedNow = $derived(f2b.jails.reduce((n, j) => n + j.currently_failed, 0));

  // ---- install ----
  let preset = $state<Preset>('norm');
  let confirmInstall = $state(false);
  let installing = $state(false);
  const presetLine = (p: Preset): string => {
    const v = PRESETS[p];
    return t('sec.f2b.presetLine', { n: v.maxretry, min: v.findtime / 60, ban: durationText(v.bantime, t) });
  };
  const installCommands = $derived.by(() => {
    const v = PRESETS[preset];
    const ign = ['127.0.0.1/8', '::1', sec.self_ip].filter(Boolean).join(' ');
    return ['apt-get install -y fail2ban python3-systemd', `# /etc/fail2ban/jail.d/linager.conf  [sshd]  maxretry=${v.maxretry}  findtime=${v.findtime}s  bantime=${v.bantime}s  ignoreip=${ign}`, 'systemctl enable fail2ban && systemctl restart fail2ban'].join('\n');
  });

  // ---- manual ban ----
  let banIp = $state('');
  let banDuration = $state('0');
  let banBusy = $state(false);
  let banError = $state('');
  let banDone = $state('');
  const durations = $derived([{ v: '0', t: t('sec.f2b.manual.default') }, { v: '3600', t: t('sec.f2b.manual.h1') }, { v: '86400', t: t('sec.f2b.manual.h24') }, { v: '604800', t: t('sec.f2b.manual.d7') }]);

  async function submitBan(): Promise<void> {
    const ip = banIp.trim();
    if (!ip || banBusy) return;
    banBusy = true;
    banError = '';
    banDone = '';
    try {
      await serverApi2.securityBan(serverId, ip, Number(banDuration));
      banDone = t('sec.f2b.banned.done', { ip });
      banIp = '';
      await onchanged();
    } catch (e) {
      banError = errorText(e);
    } finally {
      banBusy = false;
    }
  }

  let settingsOpen = $state(false);
  let bansOpen = $state(false);
  const summary = $derived(settings ? t('sec.f2b.summary', { n: settings.maxretry, min: Math.round(settings.findtime / 60), ban: durationText(settings.bantime, t) }) : '');
</script>

<section class="panel">
  <div class="row f2h">
    <h4 class="ph" style="margin:0">fail2ban</h4>
    {#if f2b.installed && f2b.active}<span class="badge b-ok">{t('sec.f2b.running')}</span>{#if summary}<span class="dim">sshd · {summary}</span>{/if}
    {:else if f2b.installed}<span class="badge b-warn">{t('sec.f2b.stoppedBadge')}</span>
    {:else}<span class="badge b-warn">{t('sec.f2b.notInstalled')}</span>{/if}
    <span class="grow"></span>
    {#if f2b.installed && f2b.active}
      <button class="btn sm" type="button" disabled={unreachable} onclick={() => (bansOpen = true)}>{t('sec.f2b.list')}{#if bannedNow > 0} <span class="badge b-crit">{bannedNow}</span>{/if}</button>
      <button class="btn sm" type="button" disabled={unreachable || !settings} onclick={() => (settingsOpen = true)}>{t('sec.f2b.settings')}</button>
    {/if}
  </div>

  {#if !f2b.installed}
    <div class="stats">
      <div class="stat"><b class="crit-t">{sec.failures.total.toLocaleString()}</b><span>{t('sec.f2b.failuresWeek')}</span></div>
      <div class="stat"><b>{sec.failures.addresses.toLocaleString()}</b><span>{t('sec.f2b.addresses')}</span></div>
    </div>
    <p class="dim">{t('sec.f2b.why')}</p>
    <div class="sh">{t('sec.f2b.how')}</div>
    <div class="opts presets">
      {#each ['soft', 'norm', 'hard'] as p (p)}
        <button class="optc" type="button" aria-pressed={preset === p} onclick={() => (preset = p as Preset)}>{t(`sec.f2b.preset.${p}`)}{#if p === 'norm'} <span class="badge b-ok">{t('sec.f2b.recommended')}</span>{/if}<small>{presetLine(p as Preset)}</small></button>
      {/each}
    </div>
    <p class="dim">{#if sec.self_ip}{t('sec.f2b.selfNote', { ip: sec.self_ip })} {/if}{t('sec.f2b.laterNote')}</p>
    {#if !confirmInstall}
      <div class="row end"><button class="btn primary" type="button" disabled={unreachable} onclick={() => (confirmInstall = true)}>{t('sec.f2b.install')}</button></div>
    {:else}
      <div class="warnbox warn">
        <p>{t('sec.f2b.installText', { preset: t(`sec.f2b.preset.${preset}`) })}</p>
        <pre class="pv">{installCommands}</pre>
        <p class="dim">{t('sec.f2b.installNote')}</p>
        <div class="row end"><button class="btn" type="button" onclick={() => (confirmInstall = false)}>{t('common.cancel')}</button>
          <button class="btn primary" type="button" onclick={() => { confirmInstall = false; installing = true; }}>{t('sec.f2b.installGo')}</button></div>
      </div>
    {/if}
  {:else if !f2b.active}
    <p class="dim">{t('sec.f2b.stopped')}</p>
  {:else}
    <div class="stats">
      <button class="stat link" type="button" onclick={() => (bansOpen = true)}><b class:crit-t={bannedNow > 0}>{bannedNow}</b><span>{t('sec.f2b.statNow')}</span></button>
      <div class="stat"><b>{totalBanned.toLocaleString()}</b><span>{t('sec.f2b.statTotal')}</span></div>
      <div class="stat"><b>{failedNow}</b><span>{t('sec.f2b.statWindow')}</span></div>
    </div>
    {#if !f2b.jails.some((j) => j.name === 'sshd')}<div class="banner">{t('sec.f2b.noSshd')}</div>{/if}
    <form class="row" onsubmit={(e) => { e.preventDefault(); void submitBan(); }}>
      <b class="small">{t('sec.f2b.manual')}</b>
      <span class="field"><input class="mono" autocomplete="off" spellcheck="false" placeholder="198.51.100.7" aria-label={t('sec.f2b.manual')} size="16" bind:value={banIp} oninput={() => { banError = ''; banDone = ''; }} /></span>
      <CustomSelect value={banDuration} options={durations} onchange={(v) => (banDuration = v)} />
      <button class="btn danger" type="submit" disabled={banBusy || unreachable || !banIp.trim()}>{t('sec.f2b.manual.go')}</button>
    </form>
    <p class="dim">{t('sec.f2b.manual.self')}</p>
    {#if banError}<div class="err" role="alert">{banError}</div>{:else if banDone}<div class="ok-t">{banDone}</div>{/if}
  {/if}
</section>

{#if installing}
  <StreamModal title={t('sec.f2b.installTitle')} {serverId} kind="f2b-install" params={{ preset }} onend={() => void onchanged()} onclose={() => (installing = false)} />
{/if}
{#if settingsOpen && settings}<F2bSettingsModal {serverId} {settings} selfIp={sec.self_ip ?? ''} onsaved={onchanged} onclose={() => (settingsOpen = false)} />{/if}
{#if bansOpen}<F2bBansModal {serverId} bans={f2b.bans} {onchanged} onclose={() => (bansOpen = false)} />{/if}
