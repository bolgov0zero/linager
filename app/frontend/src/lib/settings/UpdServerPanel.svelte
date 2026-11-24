<script lang="ts">
  import type { UpdateStep, UpdatesInfo } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import { fmtDateTime, fmtDuration, SERVER_STEPS } from './upd-format';

  let { info, busy, restarting, checking, channelBusy, oncheck, onchannel, onupdate }: {
    info: UpdatesInfo;
    /** Server update running or being started. */
    busy: boolean;
    restarting: boolean;
    checking: boolean;
    channelBusy: boolean;
    oncheck: () => void;
    onchannel: (channel: 'stable' | 'beta') => void;
    onupdate: (version: string) => void;
  } = $props();

  let confirmOpen = $state(false);
  let notesOpen = $state(false);

  const srv = $derived(info.server);
  const run = $derived(srv.update_run);
  const available = $derived(srv.available);
  const canUpdate = $derived(Boolean(available) && srv.can_update);
  const noUpdater = $derived(Boolean(available) && !srv.can_update);

  // The progress of a server update is a dialog: it opens when the update is confirmed (or a run is seen going on), and
  // after a good result it closes by itself in 5 seconds or by the OK button.
  const CLOSE_AFTER_MS = 5000;
  let statusOpen = $state(false);
  /** The run that was the latest before the update was confirmed: its state is not the one of the new update. */
  let staleRun = $state('');
  /** The run whose dialog the person closed; it is not opened again. */
  let dismissedRun = $state('');
  const freshRun = $derived(run.run_id !== staleRun);
  const finished = $derived(freshRun && (run.state === 'done' || run.state === 'failed' || run.state === 'rolled_back'));
  const working = $derived(!finished);

  $effect(() => {
    if ((run.state === 'running' || restarting) && run.run_id !== dismissedRun) statusOpen = true;
  });
  $effect(() => {
    if (!statusOpen || !finished || run.state !== 'done') return;
    const timer = setTimeout(closeStatus, CLOSE_AFTER_MS);
    return () => clearTimeout(timer);
  });
  // the update did not start (the request failed): nothing to show
  $effect(() => {
    if (statusOpen && !busy && !restarting && !freshRun) statusOpen = false;
  });
  function closeStatus(): void {
    statusOpen = false;
    dismissedRun = run.run_id;
  }
  function stepState(id: UpdateStep['id']): UpdateStep['state'] {
    return freshRun ? (run.steps?.find((s) => s.id === id)?.state ?? 'pending') : 'pending';
  }

  // The server is stopped by the update for a few seconds, so for that time nothing can tell which step is going on: the
  // steps that were not finished at that moment are shown as "going on", and the real result appears when it is back.
  let now = $state(Date.now());
  let restartSince = $state(0);
  $effect(() => {
    if (!restarting) {
      restartSince = 0;
      return;
    }
    restartSince = now = Date.now();
    const timer = setInterval(() => (now = Date.now()), 1000);
    return () => clearInterval(timer);
  });
  const waiting = $derived(restarting && freshRun && !finished);
  const waitedSec = $derived(restartSince ? Math.max(0, Math.round((now - restartSince) / 1000)) : 0);
  function shownState(id: UpdateStep['id']): UpdateStep['state'] | 'unknown' {
    const st = stepState(id);
    return waiting && st !== 'done' && st !== 'failed' ? 'unknown' : st;
  }
  /** How long a finished step took ("" while it has not finished or the times are not known). */
  function took(id: UpdateStep['id']): string {
    const step = freshRun ? run.steps?.find((s) => s.id === id) : undefined;
    if (!step || (step.state !== 'done' && step.state !== 'failed') || !step.started_ms || !step.finished_ms) return '';
    return fmtDuration(step.finished_ms - step.started_ms);
  }

  const notes = $derived(srv.notes ?? []);
  const notesVersion = $derived(srv.notes_version ?? available ?? srv.current);
  const channelOptions = [{ v: 'stable', t: 'stable' }, { v: 'beta', t: 'beta' }];

  function confirmUpdate(): void {
    confirmOpen = false;
    if (!available) return;
    staleRun = run.run_id;
    dismissedRun = '';
    statusOpen = true;
    onupdate(available);
  }
</script>

<section class="panel">
  <h4 class="ph">{t('st.upd.srv.title')}</h4>
  <div class="row" style="margin-bottom:10px">
    <b class="ubig mono">{srv.current}</b>
    {#if available}<span class="badge b-warn">{t('st.upd.srv.available', { version: available })}</span>{:else}<span class="badge b-ok">{t('st.upd.srv.current')}</span>{/if}
  </div>
  <dl class="facts">
    <div><dt>{t('st.upd.srv.channel')}</dt><dd><CustomSelect small value={info.channel} options={channelOptions} onchange={(v) => (busy || channelBusy ? undefined : onchannel(v as 'stable' | 'beta'))} /></dd></div>
    <div><dt>{t('st.upd.srv.checked')}</dt><dd>{fmtDateTime(srv.checked_at)}</dd></div>
    <div><dt>{t('st.upd.srv.migrations')}</dt><dd>{srv.db_migrations.pending > 0 ? t('st.upd.srv.migrationsPending') : t('st.upd.srv.migrationsOk')}</dd></div>
    <div><dt>{t('st.upd.srv.backup')}</dt><dd>{srv.last_backup ? fmtDateTime(srv.last_backup) : '—'}</dd></div>
    <div><dt>{t('st.upd.srv.source')}</dt><dd class="mono">{srv.source}</dd></div>
  </dl>
  {#if srv.check_error}<div class="err" style="margin-top:10px">{srv.check_error}</div>{/if}
  <div class="row" style="margin-top:12px;flex-wrap:wrap">
    <button class="btn sm" type="button" disabled={busy || checking} onclick={oncheck}>{checking ? t('st.upd.srv.checking') : t('st.upd.srv.check')}</button>
    <button class="btn sm" type="button" disabled={notes.length === 0} onclick={() => (notesOpen = true)}>{t('st.upd.notes.button')}</button>
    {#if canUpdate}<button class="btn sm primary" type="button" disabled={busy} onclick={() => (confirmOpen = true)}>{t('st.upd.srv.update', { version: available ?? '' })}</button>{/if}
  </div>
  {#if noUpdater}<p class="dim" style="margin-top:8px">{t('st.upd.srv.noUpdater')}</p>{/if}
</section>

{#if statusOpen}
  <Modal title={t('st.upd.status.title')} onclose={closeStatus}>
    <div class="mdl-stack" aria-label={t('st.upd.srv.progressAria')}>
      {#if waiting}
        <p class="dim" role="status">{t('st.upd.restarting')} {t('st.upd.restart.elapsed', { n: waitedSec })}</p>
        <p class="dim">{t('st.upd.restart.note')}</p>
      {:else if restarting}<p class="dim">{t('st.upd.restarting')}</p>{/if}
      <ol class="ustep">
        {#each SERVER_STEPS as id (id)}
          {@const st = shownState(id)}
          {@const dur = took(id)}
          <li class="s-{st}">
            <span class="ico" aria-hidden="true">{#if st === 'done'}<Icon name="check" size={12} />{:else if st === 'running'}<span class="spin"><i></i><i></i></span>{:else if st === 'unknown'}<Icon name="ellipsis" size={12} />{:else if st === 'failed'}<Icon name="x" size={12} />{:else}<Icon name="minus" size={12} />{/if}</span>
            <span>{t(`st.upd.srvstep.${id}`)}</span>
            {#if dur}<span class="dim mono">{dur}</span>{/if}
          </li>
        {/each}
      </ol>
      {#if finished && run.state === 'done'}<p class="okt">{t('st.upd.run.done', { version: run.to })}</p>
      {:else if finished && run.state === 'failed'}<div class="err">{t('st.upd.run.failed', { error: run.error ?? '' })}</div>
      {:else if finished && run.state === 'rolled_back'}<div class="err">{t('st.upd.run.rolledBack', { error: run.error ?? '' })}</div>{/if}
    </div>
    {#snippet foot()}<span class="spacer"></span><button class="btn primary" type="button" disabled={working} onclick={closeStatus}>{t('st.upd.status.ok')}</button>{/snippet}
  </Modal>
{/if}

{#if notesOpen}
  <Modal title={t('st.upd.notes.title', { version: notesVersion })} onclose={() => (notesOpen = false)}>
    {#if notes.length > 0}<ul class="upd-notes">{#each notes as note, i (i)}<li>{note}</li>{/each}</ul>{:else}<p class="dim">{t('st.upd.notes.empty')}</p>{/if}
    {#snippet foot()}<span class="spacer"></span><button class="btn" type="button" onclick={() => (notesOpen = false)}>{t('st.upd.notes.close')}</button>{/snippet}
  </Modal>
{/if}

{#if confirmOpen && available}
  <Modal title={t('st.upd.confirm.title', { version: available })} onclose={() => (confirmOpen = false)}>
    <p>{t('st.upd.confirm.text1')}</p>
    <p>{t('st.upd.confirm.text2')}</p>
    <p>{t('st.upd.confirm.text3')}</p>
    {#snippet foot()}<span class="spacer"></span><button class="btn" type="button" onclick={() => (confirmOpen = false)}>{t('st.upd.cancel')}</button><button class="btn primary" type="button" onclick={confirmUpdate}>{t('st.upd.confirm.ok')}</button>{/snippet}
  </Modal>
{/if}
