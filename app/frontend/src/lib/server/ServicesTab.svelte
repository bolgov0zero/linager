<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError, serverApi, type ServerDetail, type ServiceAction, type ServiceUnit } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import Icon from '../Icon.svelte';
  import SearchField from '../SearchField.svelte';
  import { t } from '../i18n.svelte';
  import { hasModule } from '../me.svelte';
  import { showToast } from '../toast';

  let { s }: { s: ServerDetail } = $props();

  let list = $state<ServiceUnit[]>([]);
  let loaded = $state(false);
  let error = $state('');
  let q = $state('');
  let filter = $state<'all' | 'running' | 'stopped' | 'failed'>('all');
  let ignored = $state<string[]>([]);
  let busy = $state('');
  // an operator who was given the module can use the unit buttons; ignoring a unit changes the status of the server
  const canAct = hasModule('services');

  async function load(): Promise<void> {
    try {
      list = (await serverApi.services(s.id)).services;
      void serverApi.servicesIgnored(s.id).then((r) => (ignored = r.ignored)).catch(() => undefined);
      error = '';
    } catch {
      error = t('sp.err.load');
    } finally {
      loaded = true;
    }
  }
  onMount(() => void load());

  const matches = (u: ServiceUnit): boolean => filter === 'all' || (filter === 'running' ? u.active === 'active' : filter === 'failed' ? u.active === 'failed' : u.active !== 'active' && u.active !== 'failed');
  const shown = $derived(list.filter((u) => matches(u) && (!q.trim() || `${u.name} ${u.description}`.toLowerCase().includes(q.trim().toLowerCase()))));
  const tone = (u: ServiceUnit): string => (u.active === 'failed' ? 'b-crit' : u.active === 'active' ? 'b-ok' : 'b-mute');

  async function setIgnored(u: ServiceUnit, ignore: boolean): Promise<void> {
    try {
      await serverApi.serviceIgnore(s.id, u.name, ignore);
      ignored = ignore ? [...ignored, u.name] : ignored.filter((x) => x !== u.name);
      showToast(t(ignore ? 'svc.ignoredDone' : 'svc.unignoredDone', { unit: u.name }));
    } catch {
      error = t('sp.err.action');
    }
  }

  async function act(u: ServiceUnit, a: ServiceAction): Promise<void> {
    if (busy) return;
    const danger = a === 'stop' || a === 'restart' || a === 'disable';
    if (danger && !(await confirmAsk({ title: t(`sp.svc.${a}`), text: t('sp.svc.confirm', { name: u.name, server: s.name }), ok: t(`sp.svc.${a}`), danger: a !== 'restart' }))) return;
    busy = u.name;
    try {
      try {
        await serverApi.serviceAction(s.id, u.name, a, danger);
      } catch (e) {
        // the server says this unit is a dangerous one (ssh, the network, the agent): ask once more, then repeat
        if (e instanceof ApiError && e.status === 409 && e.code === 'dangerous') {
          if (!(await confirmAsk({ title: t('sp.svc.dangerous'), text: t('sp.svc.dangerous.text', { name: u.name }), ok: t(`sp.svc.${a}`), danger: true }))) return;
          await serverApi.serviceAction(s.id, u.name, a, true);
        } else throw e;
      }
      await load();
    } catch {
      error = t('sp.err.action');
    } finally {
      busy = '';
    }
  }
</script>

<div class="sp-fill">
<div class="sp-tools">
  <SearchField placeholder={t('sp.filter')} bind:value={q} />
  <div class="seg sm">{#each ['all', 'running', 'stopped', 'failed'] as f (f)}<button type="button" aria-pressed={filter === f} onclick={() => (filter = f as typeof filter)}>{t(`svc.filter.${f}`)}</button>{/each}</div>
  <span class="dim">{t('svc.count', { n: shown.length })}</span>
  <span class="grow"></span>
  <button class="btn sm icon" type="button" title={t('sp.refresh')} onclick={load}><Icon name="refresh-cw" size={14} /></button>
</div>
{#if error}<div class="err">{error}</div>{/if}
{#if !loaded}<p class="dim">{t('dash.loading')}</p>{:else}
  <div class="panel flush fill"><table class="tbl compact fixed">
    <thead><tr><th style="width:30%">{t('sp.svc.name')}</th><th>{t('sp.svc.desc')}</th><th style="width:170px">{t('sp.svc.state')}</th><th style="width:100px">{t('sp.svc.autostart')}</th><th style="width:{canAct ? 160 : 96}px"></th></tr></thead>
    <tbody>
      {#each shown as u (u.name)}
        <tr>
          <td class="mono" title={u.name}>{u.name}{#if ignored.includes(u.name)} <span class="badge b-mute" title={t('svc.ignoreHint')}>{t('svc.badge.ignored')}</span>{/if}</td><td class="dim" title={u.description}>{u.description}</td>
          <td><span class="badge {tone(u)}">{u.active}{u.sub && u.sub !== u.active ? ` · ${u.sub}` : ''}</span></td>
          <td><span class="badge b-mute">{u.enabled}</span></td>
          <td class="r"><span class="r-btns">
            {#if canAct}
              {#if ignored.includes(u.name)}
                <button class="btn icon sm" type="button" title={t('svc.act.unignore')} onclick={() => setIgnored(u, false)}><Icon name="eye" size={14} /></button>
              {:else}
                <button class="btn icon sm" type="button" title={t('svc.act.ignore')} onclick={() => setIgnored(u, true)}><Icon name="eye-off" size={14} /></button>
              {/if}
              {#if u.enabled === 'enabled' || u.enabled === 'disabled'}
                <button class="btn icon sm" type="button" title={t(u.enabled === 'enabled' ? 'svc.act.disable' : 'svc.act.enable')} disabled={!!busy} onclick={() => act(u, u.enabled === 'enabled' ? 'disable' : 'enable')}><Icon name={u.enabled === 'enabled' ? 'toggle-right' : 'toggle-left'} size={14} /></button>
              {/if}
            {/if}
            {#if !canAct}{:else if u.active === 'active'}
              <button class="btn icon sm warn" type="button" title={t('sp.svc.restart')} disabled={!!busy} onclick={() => act(u, 'restart')}><Icon name="rotate-ccw" size={14} /></button>
              <button class="btn icon sm danger" type="button" title={t('sp.svc.stop')} disabled={!!busy} onclick={() => act(u, 'stop')}><Icon name="square" size={14} /></button>
            {:else}
              <button class="btn icon sm" type="button" title={t('sp.svc.start')} disabled={!!busy} onclick={() => act(u, 'start')}><Icon name="play" size={14} /></button>
            {/if}
          </span></td>
        </tr>
      {:else}
        <tr><td colspan="5" class="dim">{t('dash.nothing')}</td></tr>
      {/each}
    </tbody>
  </table></div>
{/if}
</div>
